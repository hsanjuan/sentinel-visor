package main

import (
	"context"
	"errors"
	"math"
	"math/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/sentinel-visor/lens"
	"github.com/filecoin-project/sentinel-visor/model/derived"
	messagemodel "github.com/filecoin-project/sentinel-visor/model/messages"
	messagetask "github.com/filecoin-project/sentinel-visor/tasks/message"
)

func runMessages(ctx context.Context, node lens.API, curChain *chain) error {
	logger.Info("Messages")
	blockMessagesModel := make(messagemodel.BlockMessages, 0, 100)

	totalGasLimit := int64(0)
	totalUniqGasLimit := int64(0)
	seen := make(map[cid.Cid]struct{})

	// read current block messages for block messages
	for _, blk := range curChain.cur.Cids() { // for each block in this tipset
		blkMsgs, err := node.ChainGetBlockMessages(ctx, blk)
		if err != nil {
			logger.Error()
			return err
		}
		for _, m := range blkMsgs.BlsMessages {
			blockMessagesModel = append(blockMessagesModel, &messagemodel.BlockMessage{
				Height:  int64(curChain.cur.Height()),
				Block:   blk.String(),
				Message: m.Cid().String(),
			})
			totalGasLimit += m.GasLimit
			if _, ok := seen[m.Cid()]; !ok {
				totalUniqGasLimit += m.GasLimit
			}
			seen[m.Cid()] = struct{}{}
		}
		for _, m := range blkMsgs.SecpkMessages {
			blockMessagesModel = append(blockMessagesModel, &messagemodel.BlockMessage{
				Height:  int64(curChain.cur.Height()),
				Block:   blk.String(),
				Message: m.Cid().String(),
			})
			totalGasLimit += m.Message.GasLimit
			if _, ok := seen[m.Message.Cid()]; !ok {
				totalUniqGasLimit += m.Message.GasLimit
			}
			seen[m.Message.Cid()] = struct{}{}
		}
	}

	curChain.dbOps = append(curChain.dbOps, blockMessagesModel)

	logger.Info("Message Gas Economy")
	parentBaseFee := curChain.cur.Blocks()[0].ParentBaseFee
	newBaseFee := store.ComputeNextBaseFee(parentBaseFee, totalUniqGasLimit, len(curChain.cur.Blocks()), curChain.cur.Height())
	baseFeeRat := new(big.Rat).SetFrac(newBaseFee.Int, new(big.Int).SetUint64(build.FilecoinPrecision))
	baseFee, _ := baseFeeRat.Float64()

	baseFeeChange := new(big.Rat).SetFrac(newBaseFee.Int, parentBaseFee.Int)
	baseFeeChangeF, _ := baseFeeChange.Float64()

	curChain.dbOps = append(curChain.dbOps, &messagemodel.MessageGasEconomy{
		Height: int64(curChain.cur.Height()),
		// FIXME
		StateRoot:           curChain.cur.ParentState().String(),
		GasLimitTotal:       totalGasLimit,
		GasLimitUniqueTotal: totalUniqGasLimit,
		BaseFee:             baseFee,
		BaseFeeChangeLog:    math.Log(baseFeeChangeF) / math.Log(1.125),
		GasFillRatio:        float64(totalGasLimit) / float64(len(curChain.cur.Blocks())*build.BlockGasTarget),
		GasCapacityRatio:    float64(totalUniqGasLimit) / float64(len(curChain.cur.Blocks())*build.BlockGasTarget),
		GasWasteRatio:       float64(totalGasLimit-totalUniqGasLimit) / float64(len(curChain.cur.Blocks())*build.BlockGasTarget),
	})

	receiptsModel := make(messagemodel.Receipts, 0, 100)
	parentMessages := make(map[cid.Cid]*types.Message)
	receipts := make(map[cid.Cid]*types.MessageReceipt)
	// messages from last block for receipts, gas outputs etc
	logger.Info("Receipts")
	for _, blk := range curChain.cur.Cids() {
		recs, err := node.ChainGetParentReceipts(ctx, blk)
		if err != nil {
			logger.Error(err)
			return err
		}
		msgs, err := node.ChainGetParentMessages(ctx, blk)
		if err != nil {
			logger.Error(err)
			return err
		}

		for i, r := range recs {
			parentMessages[msgs[i].Cid] = msgs[i].Message
			receipts[msgs[i].Cid] = r
			receiptsModel = append(receiptsModel, &messagemodel.Receipt{
				Height:    int64(curChain.cur.Height()),
				Message:   msgs[i].Cid.String(),
				StateRoot: curChain.cur.ParentState().String(),
				Idx:       i,
				ExitCode:  int64(r.ExitCode),
				GasUsed:   r.GasUsed,
			})
		}
	}
	curChain.dbOps = append(curChain.dbOps, receiptsModel)

	messagesModel := make(messagemodel.Messages, 0, 100)
	parsedMessagesModel := make(messagemodel.ParsedMessages, 0, 100)
	gasOutputsModel := make(derived.GasOutputsList, 0, 100)

	for mcid, message := range parentMessages {
		totalUniqGasLimit += message.GasLimit

		var msgSize int
		if b, err := message.Serialize(); err == nil {
			msgSize = len(b)
		} else {
			return xerrors.Errorf("serialize message: %w", err)
		}

		msgModel := &messagemodel.Message{
			Height:     int64(curChain.prev.Height()),
			Cid:        mcid.String(),
			From:       message.From.String(),
			To:         message.To.String(),
			Value:      message.Value.String(),
			GasFeeCap:  message.GasFeeCap.String(),
			GasPremium: message.GasPremium.String(),
			GasLimit:   message.GasLimit,
			SizeBytes:  msgSize,
			Nonce:      message.Nonce,
			Method:     uint64(message.Method),
		}

		// // record all unique messages
		messagesModel = append(messagesModel, msgModel)

		dstActorCode := accountActorCodeID
		// remember messages are from previous block
		dstActor, err := curChain.prevState.GetActor(message.To)
		if err != nil {
			// implicitly if actor does not exist,
			if !errors.Is(err, types.ErrActorNotFound) {
				logger.Error(err)
				return err
			}
		} else {
			dstActorCode = dstActor.Code.String()
		}
		pm, err := messagetask.ParseMsg(message, curChain.prev, dstActorCode)
		if err != nil {
			logger.Error(err)
			return err
		}
		parsedMessagesModel = append(parsedMessagesModel, pm)

		r := receipts[mcid]

		outputs := node.ComputeGasOutputs(r.GasUsed, message.GasLimit, parentBaseFee, message.GasFeeCap, message.GasPremium)

		// GasOutputs
		gasOutputsModel = append(gasOutputsModel, &derived.GasOutputs{
			Cid:                msgModel.Cid,
			From:               msgModel.From,
			To:                 msgModel.To,
			Value:              msgModel.Value,
			GasFeeCap:          message.GasFeeCap.String(),
			GasPremium:         message.GasPremium.String(),
			GasLimit:           message.GasLimit,
			SizeBytes:          msgSize,
			Nonce:              message.Nonce,
			Method:             uint64(message.Method),
			StateRoot:          curChain.prev.ParentState().String(),
			ExitCode:           int64(r.ExitCode),
			GasUsed:            receipts[mcid].GasUsed,
			ParentBaseFee:      parentBaseFee.String(),
			BaseFeeBurn:        outputs.BaseFeeBurn.String(),
			OverEstimationBurn: outputs.OverEstimationBurn.String(),
			MinerPenalty:       outputs.MinerPenalty.String(),
			MinerTip:           outputs.MinerTip.String(),
			Refund:             outputs.Refund.String(),
			GasRefund:          outputs.Refund.Int64(),
			GasBurned:          outputs.GasBurned,
		})
	}
	curChain.dbOps = append(curChain.dbOps, messagesModel, parsedMessagesModel, gasOutputsModel)
	return nil
}
