package main

import (
	"context"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/sentinel-visor/lens"
	minermodel "github.com/filecoin-project/sentinel-visor/model/actors/miner"
	"github.com/filecoin-project/sentinel-visor/tasks/actorstate"
	minerextractor "github.com/filecoin-project/sentinel-visor/tasks/actorstate"
	"golang.org/x/xerrors"
)

type MinerTaskResults struct {
	MinerInfoModel           minermodel.MinerInfoList
	FeeDebtModel             minermodel.MinerFeeDebtList
	LockedFundsModel         minermodel.MinerLockedFundsList
	CurrentDeadlineInfoModel minermodel.MinerCurrentDeadlineInfoList
	PreCommitsModel          minermodel.MinerPreCommitInfoList
	SectorsModel             minermodel.MinerSectorInfoList
	SectorEventsModel        minermodel.MinerSectorEventList
	SectorDealsModel         minermodel.MinerSectorDealList
}

func extractMiner(ctx context.Context, node lens.API, chain *chain, actor *types.Actor, info actorstate.ActorInfo, results *MinerTaskResults) error {
	//logger.Info("cur actor state")
	curState, err := miner.Load(node.Store(), actor)
	if err != nil {
		logger.Error(err)
	}
	prevState := curState
	if info.Epoch > 0 {
		//logger.Info("prev actor state")
		prevMinerActor, err := chain.prevState.GetActor(info.Address)
		if err != nil {
			return err
		}
		prevState, err = miner.Load(node.Store(), prevMinerActor)
		if err != nil {
			return nil
		}
	}

	ec := &minerextractor.MinerStateExtractionContext{
		PrevState: prevState,
		CurrActor: actor,
		CurrState: curState,
		CurrTs:    chain.cur,
	}

	logger.Info("extract miner info ", info.Address)
	minerInfoModel, err := minerextractor.ExtractMinerInfo(info, ec)
	if err != nil {
		return xerrors.Errorf("extracting miner info: %w", err)
	}

	logger.Info("extract miner locked funds")
	lockedFundsModel, err := minerextractor.ExtractMinerLockedFunds(info, ec)
	if err != nil {
		return xerrors.Errorf("extracting miner locked funds: %w", err)
	}

	logger.Info("extract miner fee debt")
	feeDebtModel, err := minerextractor.ExtractMinerFeeDebt(info, ec)
	if err != nil {
		return xerrors.Errorf("extracting miner fee debt: %w", err)
	}

	logger.Info("extract miner deadline info")
	currDeadlineModel, err := minerextractor.ExtractMinerCurrentDeadlineInfo(info, ec)
	if err != nil {
		return xerrors.Errorf("extracting miner current deadline info: %w", err)
	}

	logger.Info("extract sector data")
	preCommitModel, sectorModel, sectorDealsModel, sectorEventsModel, err := minerextractor.ExtractMinerSectorData(ctx, ec, info, node)
	if err != nil {
		return xerrors.Errorf("extracting miner sector changes: %w", err)
	}
	logger.Info("finish extract")
	// posts, err := minerextractor.ExtractMinerPoSts(ctx, &info, ec, node)
	// if err != nil {
	// 	return xerrors.Errorf("extracting miner posts: %v", err)
	// }

	results.MinerInfoModel = append(results.MinerInfoModel, minerInfoModel)
	results.LockedFundsModel = append(results.LockedFundsModel, lockedFundsModel)
	results.FeeDebtModel = append(results.FeeDebtModel, feeDebtModel)
	results.CurrentDeadlineInfoModel = append(results.CurrentDeadlineInfoModel, currDeadlineModel)
	results.SectorDealsModel = append(results.SectorDealsModel, sectorDealsModel...)
	results.SectorEventsModel = append(results.SectorEventsModel, sectorEventsModel...)
	results.SectorsModel = append(results.SectorsModel, sectorModel...)
	results.PreCommitsModel = append(results.PreCommitsModel, preCommitModel...)
	results.MinerInfoModel = append(results.MinerInfoModel, minerInfoModel)
	return nil
}
