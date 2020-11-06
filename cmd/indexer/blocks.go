package main

import (
	"github.com/filecoin-project/sentinel-visor/lens"
	blocks "github.com/filecoin-project/sentinel-visor/model/blocks"
)

func runBlocks(node lens.API, curChain *chain) error {
	logger.Info("blocks")

	for _, bh := range curChain.cur.Blocks() {
		curChain.dbOps = append(curChain.dbOps,
			blocks.NewBlockHeader(bh),
			blocks.NewBlockParents(bh),
			blocks.NewDrandBlockEntries(bh),
		)
	}
	return nil
}
