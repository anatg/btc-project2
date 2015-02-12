//
// miner.go
// Staring template for PointCoint miner.
//
// cs4501: Cryptocurrency Cafe
// University of Virginia, Spring 2015
// Project 2
//

package main

import (
	"fmt"
	"log"
	"math/big"
	"math/rand"

	"github.com/PointCoin/btcjson"
	"github.com/PointCoin/btcutil"
	"github.com/PointCoin/pointcoind/blockchain"
)

const (
	rpcuser = "[your username]"        // This match your rpcuser and rpcpass in pointcoind.conf
	rpcpass = "[your password]"        
	cert    = "/home/ubuntu/.pointcoind/rpc.cert" // Shouldn't need to change this
)

func main() {
	// Setup the client using application constants, die horribly if there's a problem
	client := setupRpcClient(cert, rpcuser, rpcpass)

	// Declare variables to use in our main loop
	var template *btcjson.GetBlockTemplateResult
	var difficulty big.Int

	var hashCounter int
	var err error

	var prevHash string
	var height int64

	for { // Loop forever (you may want to do something smarter!)
		// Get a new block template from pointcoind.
		log.Printf("Requesting a block template\n")
		template, err = client.GetBlockTemplate(&btcjson.TemplateRequest{})
		if err != nil {
			log.Fatal(err)
		}

		// The template returned by GetBlockTemplate provides these fields
		// that you will need to use to create a new block:

		// hash of the previous block
		prevHash = template.PreviousHash

		// difficulty target
		difficulty = formatDiff(template.Bits)

		// height of the next block (number of blocks between genesis block and next block)
		height = template.Height

		// returns the transactions from the network	
		txs := formatTransactions(template.Transactions) 

		msg := "Your computing ID" // replace with your UVa Computing ID (e.g., "dee2b")
		a := "PsVSrUSQf72X6GWFQXJPxR7WSAPVRb1gWx" // replace with the address you want mining fees to go to (or leave it like this and Nick gets them)

		coinbaseTx := CreateCoinbaseTx(height, a, msg) // address conversion moved into CreateCoinbaseTx
		txs = prepend(coinbaseTx.MsgTx(), txs)
		merkleRoot := createMerkleRoot(txs)

		// Finish the miner!

		// block := CreateBlock(prevHash, merkleRoot, difficulty, nonce, txs)

	}
}
