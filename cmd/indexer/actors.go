package main

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/sentinel-visor/lens"
	"github.com/filecoin-project/sentinel-visor/model"
	commonmodel "github.com/filecoin-project/sentinel-visor/model/actors/common"
	"github.com/filecoin-project/sentinel-visor/model/actors/market"
	minermodel "github.com/filecoin-project/sentinel-visor/model/actors/miner"
	"github.com/filecoin-project/sentinel-visor/model/actors/power"
	"github.com/filecoin-project/sentinel-visor/tasks/actorstate"
	sa0builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	sa2builtin "github.com/filecoin-project/specs-actors/v2/actors/builtin"
)

func runActors(ctx context.Context, node lens.API, curChain *chain, batchCh chan<- []model.PersistableWithTx) error {
	logger.Info("actor states")
	diff, err := state.Diff(curChain.prevState, curChain.curState)
	if err != nil {
		logger.Error(err)
		return err
	}
	actorDBs := make(commonmodel.ActorList, 0, 1000)
	stateDBs := make(commonmodel.ActorStateList, 0, 1000)
	minerTaskResults := &MinerTaskResults{
		MinerInfoModel:           make(minermodel.MinerInfoList, 0, 1000),
		FeeDebtModel:             make(minermodel.MinerFeeDebtList, 0, 1000),
		LockedFundsModel:         make(minermodel.MinerLockedFundsList, 0, 1000),
		CurrentDeadlineInfoModel: make(minermodel.MinerCurrentDeadlineInfoList, 0, 1000),
		PreCommitsModel:          make(minermodel.MinerPreCommitInfoList, 0, 1000),
		SectorsModel:             make(minermodel.MinerSectorInfoList, 0, 1000),
		SectorEventsModel:        make(minermodel.MinerSectorEventList, 0, 1000),
		SectorDealsModel:         make(minermodel.MinerSectorDealList, 0, 1000),
	}

	powerTaskResults := &PowerTaskResults{
		ChainPowerModel: make(power.ChainPowerList, 0, 100),
		ClaimStateModel: make(power.PowerActorClaimList, 0, 100),
	}

	marketTaskResults := &market.MarketTaskResult{
		Proposals: make(market.MarketDealProposals, 0, 500),
		States:    make(market.MarketDealStates, 0, 500),
	}

	for addrStr, actor := range diff {
		addr, err := address.NewFromString(addrStr)
		if err != nil {
			return err
		}

		// ast, err := node.StateReadState(ctx, addr, curChain.cur.Key())
		// if err != nil {
		// 	logger.Error(err)
		// 	return err
		// }

		// state, err := json.Marshal(ast.State)
		// if err != nil {
		// 	logger.Error(err)
		// 	return err
		// }
		actorDBs = append(actorDBs, &commonmodel.Actor{
			Height:    int64(curChain.cur.Height()),
			ID:        addrStr,
			StateRoot: curChain.cur.ParentState().String(),
			Code:      actorstate.ActorNameByCode(actor.Code),
			Head:      actor.Head.String(),
			Balance:   actor.Balance.String(),
			Nonce:     actor.Nonce,
		})

		stateDBs = append(stateDBs, &commonmodel.ActorState{
			Height: int64(curChain.cur.Height()),
			Head:   actor.Head.String(),
			Code:   actor.Code.String(),
			State:  "{}", //string(state),
		})

		info := actorstate.ActorInfo{
			Actor:           actor,
			Address:         addr,
			ParentStateRoot: curChain.cur.ParentState(),
			Epoch:           curChain.cur.Height(),
			TipSet:          curChain.cur.Key(),
			ParentTipSet:    curChain.prev.Key(),
		}

		switch actor.Code {
		case sa0builtin.StorageMinerActorCodeID,
			sa2builtin.StorageMinerActorCodeID:

			// err = extractMiner(ctx, node, curChain, &actor, info, minerTaskResults)
			// if err != nil {
			// 	return err
			// }

		case sa0builtin.StoragePowerActorCodeID,
			sa2builtin.StoragePowerActorCodeID:
			err = extractPower(ctx, node, curChain, &actor, info, powerTaskResults)
			if err != nil {
				logger.Error(err)
				return err
			}
		case sa0builtin.StorageMarketActorCodeID,
			sa2builtin.StorageMarketActorCodeID:
			err = extractMarket(ctx, node, curChain, &actor, info, marketTaskResults)
			if err != nil {
				logger.Error(err)
				return err
			}
		default:
			// Unsupported actor
		}
	}

	curChain.dbOps = append(curChain.dbOps,
		actorDBs,
		stateDBs,
		minerTaskResults.MinerInfoModel,
		minerTaskResults.LockedFundsModel,
		minerTaskResults.FeeDebtModel,
		minerTaskResults.CurrentDeadlineInfoModel,
		minerTaskResults.SectorDealsModel,
		minerTaskResults.SectorEventsModel,
		minerTaskResults.SectorsModel,
		minerTaskResults.PreCommitsModel,
		minerTaskResults.MinerInfoModel,
		powerTaskResults.ChainPowerModel,
		powerTaskResults.ClaimStateModel,
		marketTaskResults.Proposals,
		marketTaskResults.States,
	)

	// TODO: parsed actors
	logger.Infof("Inserted %d actors", len(diff))
	return nil
}
