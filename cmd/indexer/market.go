package main

import (
	"context"

	lotusmarket "github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/sentinel-visor/lens"
	"github.com/filecoin-project/sentinel-visor/model/actors/market"
	"github.com/filecoin-project/sentinel-visor/tasks/actorstate"
)

func extractMarket(ctx context.Context, node lens.API, chain *chain, actor *types.Actor, info actorstate.ActorInfo, results *market.MarketTaskResult) error {
	m := actorstate.StorageMarketExtractor{}

	pred := state.NewStatePredicates(node)
	marketPred := actorstate.StorageMarketChangesPred(pred)

	curState, err := lotusmarket.Load(node.Store(), actor)
	if err != nil {
		return err
	}

	prevActor, err := chain.prevState.GetActor(info.Address)
	if err != nil {
		return err
	}

	prevState, err := lotusmarket.Load(node.Store(), prevActor)
	if err != nil {
		logger.Error(err)
		return err
	}

	changed, val, err := marketPred(ctx, prevState, curState)
	if err != nil {
		return err
	}
	if !changed {
		return err
	}

	mchanges, ok := val.(*actorstate.MarketChanges)
	if !ok {
		return err
	}

	if mchanges.ProposalChanges != nil {
		proposals, err := m.MarketDealProposalChanges(ctx, info, mchanges.ProposalChanges)
		if err != nil {
			return err
		}
		results.Proposals = append(results.Proposals, proposals...)
	}

	if mchanges.DealChanges != nil {
		states, err := m.MarketDealStateChanges(ctx, info, mchanges.DealChanges)
		if err != nil {
			return err
		}
		results.States = append(results.States, states...)
	}
	return nil
}
