package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/mit-dci/lit-rpc-client-go"
	"github.com/mit-dci/lit/dlc"
	"github.com/mit-dci/lit/lnutil"
)

var oraclePubKey, rPoint [33]byte
var oracleSig [32]byte
var oracleValue int64
var lit1, lit2 *litrpcclient.LitRpcClient

func handleError(err error) {
	if err != nil {
		panic(err.Error())
	}
}

func main() {
	parsedBytes, err := hex.DecodeString("03c0d496ef6656fe102a689abc162ceeae166832d826f8750c94d797c92eedd465")
	handleError(err)
	copy(oraclePubKey[:], parsedBytes)

	parsedBytes, err = hex.DecodeString("027168bba1aaecce0500509df2ff5e35a4f55a26a8af7ceacd346045eceb1786ad")
	handleError(err)
	copy(rPoint[:], parsedBytes)

	oracleValue = 15161

	parsedBytes, err = hex.DecodeString("9e349c50db6d07d5d8b12b7ada7f91d13af742653ff57ffb0b554170536faeac")
	handleError(err)
	copy(oracleSig[:], parsedBytes)

	lit1, err = litrpcclient.NewClient("localhost", 8001)
	handleError(err)
	lit2, err = litrpcclient.NewClient("localhost", 8002)
	handleError(err)

	// Connect both LIT peers together
	fmt.Println("Connecting nodes together...")
	err = connectNodes()
	handleError(err)

	// Find out if the oracle is present and add it if not
	fmt.Println("Ensuring oracle is available...")
	oracleIdxs, err := checkOracle()
	handleError(err)

	// Create the contract and set its parameters
	fmt.Println("Creating the contract...")
	contract, err := createContract(oracleIdxs[0])
	handleError(err)

	// Offer the contract to the other peer
	fmt.Println("Offering the contract to the other peer...")
	lit1.OfferContract(contract.Idx, 1)

	// Wait for the contract to be exchanged
	fmt.Println("Waiting for the contract to be exchanged...")
	time.Sleep(2 * time.Second)

	// Accept the contract on the second node
	fmt.Println("Accepting the contract on the other peer...")
	err = acceptContract()
	handleError(err)

	// Wait for the contract to be activated
	fmt.Println("Waiting for the contract to be activated...")
	for {
		active, err := isContractActive(contract.Idx)
		handleError(err)
		if active {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("Contract active. Generate a block on regtest and press enter")
	var input string
	fmt.Scanln(&input)

	// Settle the contract
	fmt.Println("Settling the contract...")
	err = lit1.SettleContract(contract.Idx, oracleValue, oracleSig[:])
	handleError(err)

	fmt.Println("Contract settled. Mine two blocks to ensure contract outputs are claimed back to the nodes' wallets.\r\n\r\nDone.")

}

func connectNodes() error {
	// Instruct both nodes to listen for incoming connections
	err := lit1.Listen(":2448")
	handleError(err)
	err = lit2.Listen(":2449")
	handleError(err)

	// Connect node 1 to node 2
	lnAdr, err := lit2.GetLNAddress()
	handleError(err)
	lit1.Connect(lnAdr, "localhost", 2449)

	return nil
}

func checkOracle() ([]uint64, error) {
	// Fetch a list of oracles from both nodes
	oracles1, err := lit1.ListOracles()
	handleError(err)
	oracles2, err := lit2.ListOracles()
	handleError(err)

	// Find the oracle we need in both lists
	var oracle1, oracle2 *dlc.DlcOracle

	for _, v := range oracles1 {
		if bytes.Equal(v.A[:], oraclePubKey[:]) {
			oracle1 = v
		}
	}

	for _, v := range oracles2 {
		if bytes.Equal(v.A[:], oraclePubKey[:]) {
			oracle2 = v
		}
	}

	// If the oracle is not present on node 1, add it
	if oracle1 == nil {
		oracle1, err = lit1.AddOracle(hex.EncodeToString(oraclePubKey[:]), "Tutorial")
	}

	// If the oracle is not present on node 2, add it
	if oracle2 == nil {
		oracle2, err = lit2.AddOracle(hex.EncodeToString(oraclePubKey[:]), "Tutorial")
	}

	// Return the index the oracle has on both nodes
	return []uint64{oracle1.Idx, oracle2.Idx}, nil
}

func createContract(oracleIdx uint64) (*lnutil.DlcContract, error) {
	// Create a new empty draft contract
	contract, err := lit1.NewContract()
	handleError(err)

	// Configure the contract to use the oracle we need
	err = lit1.SetContractOracle(contract.Idx, oracleIdx)
	handleError(err)

	// Set the settlement time to June 13, 2018 midnight UTC
	err = lit1.SetContractSettlementTime(contract.Idx, 1528848000)
	handleError(err)

	// Set the coin type of the contract to Bitcoin Regtest
	err = lit1.SetContractCoinType(contract.Idx, 257)
	handleError(err)

	// Configure the contract to use the R-point we need
	err = lit1.SetContractRPoint(contract.Idx, rPoint[:])
	handleError(err)

	// Set the contract funding to 1 BTC each
	err = lit1.SetContractFunding(contract.Idx, 100000000, 100000000)
	handleError(err)

	// Configure the contract division so that we get all the
	// funds when the value is 20000, and our counter party gets
	// all the funds when the value is 10000
	err = lit1.SetContractDivision(contract.Idx, 20000, 10000)
	handleError(err)

	return contract, nil
}

func acceptContract() error {
	// Get all contracts for node 2
	contracts, err := lit2.ListContracts()
	handleError(err)

	for _, c := range contracts {
		if c.Status == lnutil.ContractStatusOfferedToMe {
			err := lit2.AcceptContract(c.Idx)
			return err
		}
	}

	return fmt.Errorf("No contract found to accept")
}

func isContractActive(contractIdx uint64) (bool, error) {
	// Fetch the contract from node 1
	contract, err := lit1.GetContract(contractIdx)
	if err != nil {
		return false, err
	}

	return (contract.Status == lnutil.ContractStatusActive), nil
}
