package main

import (
	"context"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/sentinel-visor/lens"
	"github.com/filecoin-project/sentinel-visor/model/actors/market"
	"github.com/filecoin-project/sentinel-visor/tasks/actorstate"
)

func extractMarket(ctx context.Context, node lens.API, chain *chain, actor *types.Actor, info actorstate.ActorInfo, results *market.MarketTaskResult) error {
	eae := actorstate.StorageMarketExtractor{}
	res, err := eae.Extract(ctx, info, node)
	if err != nil {
		logger.Error(err)
	}
	*results = *res.(*market.MarketTaskResult)
	return nil
}
