// Some of these functions have been shamelessly taken from https://github.com/btcsuite/btcd/blob/master/mining.go

package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"math/rand"
	"strconv"

	"github.com/PointCoin/btcjson"
	"github.com/PointCoin/btcnet"
	"github.com/PointCoin/btcrpcclient"
	"github.com/PointCoin/btcutil"
	"github.com/PointCoin/btcwire"
	"github.com/PointCoin/pointcoind/blockchain"
	"github.com/PointCoin/pointcoind/txscript"
)

// setupRpcClient handles establishing a connection to the pointcoind using
// the provided parameters. The function will cease an error if the full node
// is not running.
func setupRpcClient(cfile string, rpcuser string, rpcpass string) *btcrpcclient.Client {
	// Get the raw bytes of the certificate required by the rpcclient.
	cert, err := ioutil.ReadFile(cfile)
	if err != nil {
		s := fmt.Sprintf("setupRpcClient failed with: %s\n", err)
		log.Fatal(s)
	}

	// Setup the RPC client
	connCfg := &btcrpcclient.ConnConfig{
		Host:         "127.0.0.1:8334",
		User:         rpcuser,
		Pass:         rpcpass,
		Certificates: cert,
		// Use the websocket endpoint to keep the connection alive
		// in the event we want to do polling.
		Endpoint: "ws",
	}

	client, err := btcrpcclient.New(connCfg, nil)
	if err != nil {
		s := fmt.Sprintf("setupRpcClient failed with: %s\n", err)
		log.Fatal(s)
	}

	// Test the connection to see if we can really connect
	_, err = client.GetInfo()
	if err != nil {
		log.Fatal(err)
		s := fmt.Sprintf("setupRpcClient failed with: %s\n", err)
		log.Fatal(s)
	}

	return client
}

// lessThanDiff returns true if the hash satisifies the target difficulty. That
// is to say if the hash interpreted as a big integer is less than the required
// difficulty then return true otherwise return false.
func lessThanDiff(hash btcwire.ShaHash, difficulty big.Int) bool {
	bigI := blockchain.ShaHashToBig(&hash)
	return bigI.Cmp(&difficulty) <= 0
}

// standardCoinbaseScript returns a standard script suitable for use as the
// signature script of the coinbase transaction of a new block.  In particular,
// it starts with the block height that is required by version 2 blocks and adds
// the extra nonce as well as additional coinbase flags.
func standardCoinbaseScript(nextBlockHeight int64, extraNonce uint64, msg string) ([]byte, error) {
	return txscript.NewScriptBuilder().AddInt64(nextBlockHeight).
		AddUint64(extraNonce).AddData([]byte(msg)).Script()
}

// createCoinbaseTx returns a coinbase transaction paying an appropriate subsidy
// based on the passed block height to the provided address.  When the address
// is nil, the coinbase transaction will instead be redeemable by anyone.
func createCoinbaseTx(coinbaseScript []byte, nextBlockHeight int64, addr btcutil.Address) (*btcutil.Tx, error) {
	// Create the script to pay to the provided payment address if one was
	// specified.  Otherwise create a script that allows the coinbase to be
	// redeemable by anyone.
	var pkScript []byte
	if addr != nil {
		var err error
		pkScript, err = txscript.PayToAddrScript(addr)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		scriptBuilder := txscript.NewScriptBuilder()
		pkScript, err = scriptBuilder.AddOp(txscript.OP_TRUE).Script()
		if err != nil {
			return nil, err
		}
	}

	tx := btcwire.NewMsgTx()
	tx.AddTxIn(&btcwire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *btcwire.NewOutPoint(&btcwire.ShaHash{},
			btcwire.MaxPrevOutIndex),
		SignatureScript: coinbaseScript,
		Sequence:        btcwire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(&btcwire.TxOut{
		Value: blockchain.CalcBlockSubsidy(nextBlockHeight,
			&btcnet.MainNetParams),
		PkScript: pkScript,
	})
	return btcutil.NewTx(tx), nil
}

// Creates a coinbase transaction from the next block height, a funding address,
// and an optional (short) message. The message can be whatever you want actually.
func CreateCoinbaseTx(nextBlockHeight int64, a string, msg string) *btcutil.Tx {
	n := uint64(rand.Uint32())
	script, err := standardCoinbaseScript(nextBlockHeight, n, msg)
	if err != nil {
		s := fmt.Sprintf("CreateCoinbaseTx failed with: %s\n", err)
		log.Fatal(s)
	}
	addr, err := btcutil.DecodeAddress(a, &btcnet.MainNetParams)
	if err != nil {
		s := fmt.Sprintf("Decoding the provided address failed with: %s\n", err)
		log.Fatal(s)
	}

	tx, err := createCoinbaseTx(script, nextBlockHeight, addr)
	if err != nil {
		s := fmt.Sprintf("CreateCoinbaseTx failed with: %s\n", err)
		log.Fatal(s)
	}
	return tx
}

// foramtDiff converts the current blockchain difficulty from the format provided
// by a Block Template response to a big integer for use in comparisons
func formatDiff(bits string) big.Int {
	// Convert the difficulty bits into a unint32.
	b, err := strconv.ParseUint(bits, 16, 32)
	if err != nil {
		log.Fatal(err) // This should not fail, so die horribly if it does
	}

	// Then into a big.Int
	return *blockchain.CompactToBig(uint32(b))
}

// formatTransactions converts a list of btcjson transactions into btcwire transactions
// so that we can use them in a new block.
func formatTransactions(txs []btcjson.GetBlockTemplateResultTx) []*btcwire.MsgTx {

	msgtxs := []*btcwire.MsgTx{}
	for _, tx := range txs {
		d, _ := hex.DecodeString(tx.Data)
		msgtx := btcwire.NewMsgTx()
		msgtx.Deserialize(bytes.NewBuffer(d))
		msgtxs = append(msgtxs, msgtx)
	}

	return msgtxs
}

// Prepend adds i to the front of l.
func prepend(i *btcwire.MsgTx, l []*btcwire.MsgTx) []*btcwire.MsgTx {

	lst := []*btcwire.MsgTx{}
	lst = append(lst, i)
	for _, elem := range l {
		lst = append(lst, elem)
	}

	return lst
}

// createBlock creates a new block from the provided block template. The majority
// of the work here is interpreting the information provided by the block template.
func CreateBlock(prevHash string, merkleRoot *btcwire.ShaHash, difficulty big.Int,
	nonce uint32, txs []*btcwire.MsgTx) *btcwire.MsgBlock {
	prevH, _ := btcwire.NewShaHashFromStr(prevHash)

	d := blockchain.BigToCompact(&difficulty)
	header := btcwire.NewBlockHeader(prevH, merkleRoot, d, nonce)

	msgBlock := btcwire.NewMsgBlock(header)
	for _, tx := range txs {
		msgBlock.AddTransaction(tx)
	}

	return msgBlock
}

// createMerkleRoot takes a list of transactions and produces the root
// hash from that list. For simplicities sake it uses a library function
// which requires input in a funky format and outputs the whole merkle
// tree as a list...
func createMerkleRoot(txs []*btcwire.MsgTx) *btcwire.ShaHash {
	// Convert our txs into another tx type!
	txutil := []*btcutil.Tx{}
	for _, tx := range txs {
		convtx := btcutil.NewTx(tx)
		txutil = append(txutil, convtx)
	}

	// The merkle tree store is a reverse binary tree!
	store := blockchain.BuildMerkleTreeStore(txutil)
	merkleRoot := store[len(store)-1]
	return merkleRoot
}
