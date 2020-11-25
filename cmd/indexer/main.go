package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/sentinel-visor/commands"
	"github.com/filecoin-project/sentinel-visor/lens"
	"github.com/filecoin-project/sentinel-visor/model"
	"github.com/filecoin-project/sentinel-visor/storage"
	"github.com/filecoin-project/statediff"
	"github.com/go-pg/pg/v10"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	logging "github.com/ipfs/go-log/v2"
)

var logger = logging.Logger("visor-indexer")

var (
	accountActorCodeID string
	log                = logging.Logger("message")
)

func init() {
	for code, actor := range statediff.LotusActorCodes {
		if actor == statediff.AccountActorState {
			accountActorCodeID = code
			break
		}
	}
}

func main() {
	app := &cli.App{
		Name: "visor-indexer",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "lens",
				EnvVars: []string{"VISOR_LENS"},
				Value:   "lotusrepo",
			},
			&cli.IntFlag{
				Name:    "lens-cache-hint",
				EnvVars: []string{"VISOR_LENS_CACHE_HINT"},
				Value:   1024 * 1024,
			},
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "db",
				EnvVars: []string{"LOTUS_DB"},
				Value:   "postgres://postgres:password@localhost:5432/postgres?sslmode=disable",
			},
			&cli.IntFlag{
				Name:    "db-pool-size",
				EnvVars: []string{"LOTUS_DB_POOL_SIZE"},
				Value:   75,
			},
			&cli.Int64Flag{
				Name:    "from",
				Usage:   "Limit actor and message processing to tipsets at or above `HEIGHT`",
				Value:   -1,
				EnvVars: []string{"VISOR_HEIGHT_FROM"},
			},
			&cli.Int64Flag{
				Name:        "to",
				Usage:       "Limit actor and message processing to tipsets at or below `HEIGHT`",
				Value:       -1,
				DefaultText: "MaxInt64",
				EnvVars:     []string{"VISOR_HEIGHT_TO"},
			},
		},
		Action: func(cctx *cli.Context) error {
			logging.SetLogLevel("*", "INFO")
			heightFrom := cctx.Int64("from")
			heightTo := cctx.Int64("to")

			ctx, rctx, err := commands.SetupStorageAndAPI(cctx)
			if err != nil {
				return xerrors.Errorf("setup storage and api: %w", err)
			}

			defer func() {
				rctx.Closer()
				if err := rctx.DB.Close(ctx); err != nil {
					log.Errorw("close database", "error", err)
				}
			}()

			node, nodeCloser, err := rctx.Opener.Open(ctx)
			if err != nil {
				return xerrors.Errorf("open api: %w", err)
			}
			defer nodeCloser()

			return run(ctx, rctx.DB, node, heightFrom, heightTo)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type chain struct {
	height    uint64
	dbOps     []model.PersistableWithTx
	cur       *types.TipSet
	prev      *types.TipSet
	curState  *state.StateTree
	prevState *state.StateTree
}

func run(ctx context.Context, db *storage.Database, node lens.API, from, to int64) error {
	head, err := node.ChainHead(ctx)
	cur, err := node.ChainGetTipSet(ctx, head.Parents())
	if err != nil {
		logger.Error(err)
		return err
	}
	last := head
	if to > 0 {
		last, err = node.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(to), head.Key())
		if err != nil {
			logger.Error(err)
			return err
		}
		cur, err = node.ChainGetTipSet(ctx, last.Parents())
		if err != nil {
			logger.Error(err)
			return err
		}
	}
	if from <= 0 {
		from = 1
	}

	curState, err := state.LoadStateTree(node.Store(), cur.ParentState())
	if err != nil {
		logger.Error(err)
		return err
	}
	var prev *types.TipSet
	var prevState *state.StateTree

	var wg sync.WaitGroup
	defer wg.Wait()

	batchCount := 0
	batchCh := make(chan []model.PersistableWithTx, 10)

	exit := func() {
		close(batchCh)
		wg.Wait()
		os.Exit(0)
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func(c chan os.Signal) {
		<-c
		exit()
	}(c)

	wg.Add(15)
	for i := 0; i < 15; i++ {
		go func() {
			defer wg.Done()
			commitBatchWorker(ctx, db, batchCh)
		}()
	}

	curChain := &chain{
		height:    uint64(cur.Height()),
		dbOps:     make([]model.PersistableWithTx, 0, 100),
		cur:       cur,
		prev:      prev,
		curState:  curState,
		prevState: prevState,
	}

	for height := cur.Height(); height >= abi.ChainEpoch(from); height-- {
		logger.Infof("Height: %d", height)
		prev, err = node.ChainGetTipSet(ctx, curChain.cur.Parents())
		if err != nil {
			logger.Error(err)
			return err
		}

		prevState, err = state.LoadStateTree(node.Store(), prev.ParentState())
		if err != nil {
			logger.Error(err)
			return err
		}
		curChain.prev = prev
		curChain.prevState = prevState
		//logger.Infof("cur: %s, prev: %s", curChain.cur.Height(), curChain.prev.Height())
		//logger.Infof("curState: %s, prevState: %s", curChain.cur.ParentState(), curChain.prev.ParentState())

		if err := runBlocks(node, curChain); err != nil {
			logger.Error(err)
			return err
		}

		if err := runActors(ctx, node, curChain, batchCh); err != nil {
			logger.Error(err)
			return err
		}

		if err := runMessages(ctx, node, curChain); err != nil {
			logger.Error(err)
			return err
		}

		// Commit every 100 tipsets
		if batchCount == 20 {
			//logger.Info("insert batch")
			batchCh <- curChain.dbOps
			batchCount = 0
			curChain.dbOps = make([]model.PersistableWithTx, 0, 100)
		}

		// Update things
		batchCount++
		curChain.curState = prevState
		curChain.cur = prev
	}

	exit()
	return nil
}

func commitBatchWorker(ctx context.Context, db *storage.Database, ch <-chan []model.PersistableWithTx) {
	for dbOps := range ch {
		//logger.Infof("Inserting %d ops in transaction", len(dbOps))
		if len(dbOps) == 0 {
			continue
		}

		err := db.DB.RunInTransaction(ctx, func(tx *pg.Tx) error {

			for _, op := range dbOps {
				err := op.PersistWithTx(ctx, tx)
				if err != nil {
					return err
				}
				//logger.Info("persist ", i)
			}
			return nil
		})
		if err != nil {
			logger.Errorf("failed to commit: %s", err)
		} else {
			logger.Infof("Inserted %d ops in transaction", len(dbOps))
		}
	}
}
