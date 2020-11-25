package main

import (
	"context"

	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/sentinel-visor/lens"
	powermodel "github.com/filecoin-project/sentinel-visor/model/actors/power"
	"github.com/filecoin-project/sentinel-visor/tasks/actorstate"
	"golang.org/x/xerrors"
)

type PowerTaskResults struct {
	ChainPowerModel powermodel.ChainPowerList
	ClaimStateModel powermodel.PowerActorClaimList
}

func extractPower(ctx context.Context, node lens.API, chain *chain, actor *types.Actor, info actorstate.ActorInfo, results *PowerTaskResults) error {
	curState, err := power.Load(node.Store(), actor)
	if err != nil {
		logger.Error(err)
		return err
	}

	prevState := curState
	if info.Epoch > 0 {
		prevActor, err := chain.prevState.GetActor(info.Address)
		if err != nil {
			return err
		}

		prevState, err = power.Load(node.Store(), prevActor)
		if err != nil {
			logger.Error(err)
			return xerrors.Errorf("loading previous power actor state: %w", err)
		}
	}
	psec := &actorstate.PowerStateExtractionContext{
		PrevState: prevState,
		CurrState: curState,
		CurrTs:    chain.cur,
	}

	logger.Info("extract chain power")
	chainPowerModel, err := actorstate.ExtractChainPower(psec)
	if err != nil {
		return err
	}

	logger.Info("extract claimed power")
	claimedPowerModel, err := actorstate.ExtractClaimedPower(psec)
	if err != nil {
		return err
	}
	results.ChainPowerModel = append(results.ChainPowerModel, chainPowerModel)
	results.ClaimStateModel = append(results.ClaimStateModel, claimedPowerModel...)
	return nil
}
