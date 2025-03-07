package types

import (
	"math/big"
	"strings"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	_ OutgoingTx = &SignerSetTx{}
	_ OutgoingTx = &BatchTx{}
	_ OutgoingTx = &ContractCallTx{}
)

const (
	_ = iota
	SignerSetTxPrefixByte
	BatchTxPrefixByte
	ContractCallTxPrefixByte
)

///////////////////
// GetStoreIndex //
///////////////////

// TODO: do we need a prefix byte for the different types?
func (sstx *SignerSetTx) GetStoreIndex() []byte {
	return MakeSignerSetTxKey(sstx.Nonce)
}

func (btx *BatchTx) GetStoreIndex() []byte {
	return MakeBatchTxKey(gethcommon.HexToAddress(btx.TokenContract), btx.BatchNonce)
}

func (cctx *ContractCallTx) GetStoreIndex() []byte {
	return MakeContractCallTxKey(cctx.InvalidationScope.Bytes(), cctx.InvalidationNonce)
}

///////////////////
// GetCheckpoint //
///////////////////

func (sstx *SignerSetTx) GetCosmosHeight() uint64 {
	return sstx.Height
}

func (btx *BatchTx) GetCosmosHeight() uint64 {
	return btx.Height
}

func (cctx *ContractCallTx) GetCosmosHeight() uint64 {
	return cctx.Height
}

///////////////////
// GetCheckpoint //
///////////////////

// GetCheckpoint returns the checkpoint
func (u SignerSetTx) GetCheckpoint(gravityID []byte) []byte {
	// error case here should not occur outside of testing since the above is a constant
	contractAbi, err := abi.JSON(strings.NewReader(SignerSetTxCheckpointABIJSON))
	if err != nil {
		panic(err)
	}

	// the contract argument is not a arbitrary length array but a fixed length 32 byte
	// array, therefore we have to utf8 encode the string (the default in this case) and
	// then copy the variable length encoded data into a fixed length array. This function
	// will panic if gravityId is too long to fit in 32 bytes
	gravityIDFixed, err := byteArrayToFixByteArray(gravityID)
	if err != nil {
		panic(err)
	}

	checkpointBytes := []uint8("checkpoint")
	var checkpoint [32]uint8
	copy(checkpoint[:], checkpointBytes[:])

	memberAddresses := make([]gethcommon.Address, len(u.Signers))
	convertedPowers := make([]*big.Int, len(u.Signers))
	for i, m := range u.Signers {
		memberAddresses[i] = gethcommon.HexToAddress(m.EthereumAddress)
		convertedPowers[i] = big.NewInt(int64(m.Power))
	}
	// the word 'checkpoint' needs to be the same as the 'name' above in the checkpointAbiJson
	// but other than that it's a constant that has no impact on the output. This is because
	// it gets encoded as a function name which we must then discard.
	bytes, packErr := contractAbi.Pack(
		"checkpoint",
		gravityIDFixed,
		checkpoint,
		big.NewInt(int64(u.Nonce)),
		memberAddresses,
		convertedPowers,
	)

	// this should never happen outside of test since any case that could crash on encoding
	// should be filtered above.
	if packErr != nil {
		panic(packErr)
	}

	// we hash the resulting encoded bytes discarding the first 4 bytes these 4 bytes are the constant
	// method name 'checkpoint'. If you where to replace the checkpoint constant in this code you would
	// then need to adjust how many bytes you truncate off the front to get the output of abi.encode()
	hash := crypto.Keccak256Hash(bytes[4:])
	return hash.Bytes()
}

// GetCheckpoint gets the checkpoint signature from the given outgoing tx batch
func (b BatchTx) GetCheckpoint(gravityID []byte) []byte {

	encodedBatch, err := abi.JSON(strings.NewReader(BatchTxCheckpointABIJSON))
	if err != nil {
		panic(sdkerrors.Wrap(err, "bad ABI definition in code"))
	}

	// the contract argument is not a arbitrary length array but a fixed length 32 byte
	// array, therefore we have to utf8 encode the string (the default in this case) and
	// then copy the variable length encoded data into a fixed length array. This function
	// will panic if gravityId is too long to fit in 32 bytes
	gravityIDFixed, err := byteArrayToFixByteArray(gravityID)
	if err != nil {
		panic(err)
	}

	// Create the methodName argument which salts the signature
	methodNameBytes := []uint8("transactionBatch")
	var batchMethodName [32]uint8
	copy(batchMethodName[:], methodNameBytes[:])

	// Run through the elements of the batch and serialize them
	txAmounts := make([]*big.Int, len(b.Transactions))
	txDestinations := make([]gethcommon.Address, len(b.Transactions))
	txFees := make([]*big.Int, len(b.Transactions))
	for i, tx := range b.Transactions {
		txAmounts[i] = tx.Erc20Token.Amount.BigInt()
		txDestinations[i] = gethcommon.HexToAddress(tx.EthereumRecipient)
		txFees[i] = tx.Erc20Fee.Amount.BigInt()
	}

	// the methodName needs to be the same as the 'name' above in the checkpointAbiJson
	// but other than that it's a constant that has no impact on the output. This is because
	// it gets encoded as a function name which we must then discard.
	abiEncodedBatch, err := encodedBatch.Pack(
		"submitBatch",
		gravityIDFixed,
		batchMethodName,
		txAmounts,
		txDestinations,
		txFees,
		big.NewInt(int64(b.BatchNonce)),
		gethcommon.HexToAddress(b.TokenContract),
		big.NewInt(int64(b.Timeout)),
	)

	// this should never happen outside of test since any case that could crash on encoding
	// should be filtered above.
	if err != nil {
		panic(sdkerrors.Wrap(err, "packing checkpoint"))
	}

	// we hash the resulting encoded bytes discarding the first 4 bytes these 4 bytes are the constant
	// method name 'checkpoint'. If you where to replace the checkpoint constant in this code you would
	// then need to adjust how many bytes you truncate off the front to get the output of encodedBatch.encode()
	return crypto.Keccak256Hash(abiEncodedBatch[4:]).Bytes()
}

// GetCheckpoint gets the checkpoint signature from the given outgoing tx batch
func (c ContractCallTx) GetCheckpoint(gravityID []byte) []byte {

	encodedCall, err := abi.JSON(strings.NewReader(ContractCallTxABIJSON))
	if err != nil {
		panic(sdkerrors.Wrap(err, "bad ABI definition in code"))
	}

	// Create the methodName argument which salts the signature
	methodNameBytes := []uint8("logicCall")
	var logicCallMethodName [32]uint8
	copy(logicCallMethodName[:], methodNameBytes[:])

	// the contract argument is not a arbitrary length array but a fixed length 32 byte
	// array, therefore we have to utf8 encode the string (the default in this case) and
	// then copy the variable length encoded data into a fixed length array. This function
	// will panic if gravityId is too long to fit in 32 bytes
	gravityIDFixed, err := byteArrayToFixByteArray(gravityID)
	if err != nil {
		panic(err)
	}

	// Run through the elements of the logic call and serialize them
	transferAmounts := make([]*big.Int, len(c.Tokens))
	transferTokenContracts := make([]gethcommon.Address, len(c.Tokens))
	feeAmounts := make([]*big.Int, len(c.Fees))
	feeTokenContracts := make([]gethcommon.Address, len(c.Fees))
	for i, coin := range c.Tokens {
		transferAmounts[i] = coin.Amount.BigInt()
		transferTokenContracts[i] = gethcommon.HexToAddress(coin.Contract)
	}
	for i, coin := range c.Fees {
		feeAmounts[i] = coin.Amount.BigInt()
		feeTokenContracts[i] = gethcommon.HexToAddress(coin.Contract)
	}
	payload := make([]byte, len(c.Payload))
	copy(payload, c.Payload)
	var invalidationId [32]byte
	copy(invalidationId[:], c.InvalidationScope[:])

	// the methodName needs to be the same as the 'name' above in the checkpointAbiJson
	// but other than that it's a constant that has no impact on the output. This is because
	// it gets encoded as a function name which we must then discard.
	abiEncodedCall, err := encodedCall.Pack(
		"checkpoint",
		gravityIDFixed,
		logicCallMethodName,
		transferAmounts,
		transferTokenContracts,
		feeAmounts,
		feeTokenContracts,
		gethcommon.HexToAddress(c.Address),
		payload,
		big.NewInt(int64(c.Timeout)),
		invalidationId,
		big.NewInt(int64(c.InvalidationNonce)),
	)

	// this should never happen outside of test since any case that could crash on encoding
	// should be filtered above.
	if err != nil {
		panic(sdkerrors.Wrap(err, "packing checkpoint"))
	}

	return crypto.Keccak256Hash(abiEncodedCall[4:]).Bytes()
}
