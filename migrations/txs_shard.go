package migrations

import (
	"bytes"
	"encoding/binary"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/common/dbutils"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/rlp"
	"github.com/ledgerwatch/log/v3"
)

//var txsShard = Migration{
//	Name: "txs_shard",
//	Up: func(db kv.RwDB, tmpdir string, progress []byte, BeforeCommit Callback) (err error) {
//		tx, err := db.BeginRw(context.Background())
//		if err != nil {
//			return err
//		}
//		defer tx.Rollback()
//
//		if err := BeforeCommit(tx, nil, true); err != nil {
//			return err
//		}
//		return tx.Commit()
//	},
//}

type DeprecatedBodyForStorage struct {
	BaseTxId uint64
	TxAmount uint32
	Uncles   []*types.Header
}

func ReadBody(db kv.Getter, hash common.Hash, number uint64) (*types.Body, uint64, uint32) {
	bodyRlp, err := db.GetOne(kv.BlockBody, dbutils.BlockBodyKey(number, hash))
	if err != nil {
		log.Error("ReadBodyRLP failed", "err", err)
	}
	if len(bodyRlp) == 0 {
		return nil, 0, 0
	}
	bodyForStorage := new(DeprecatedBodyForStorage)
	if err := rlp.DecodeBytes(bodyRlp, bodyForStorage); err != nil {
		log.Error("Invalid block body RLP", "hash", hash, "err", err)
		return nil, 0, 0
	}
	body := new(types.Body)
	body.Uncles = bodyForStorage.Uncles
	return body, bodyForStorage.BaseTxId, bodyForStorage.TxAmount
}

func CanonicalTransactions(db kv.Getter, baseTxId uint64, amount uint32) ([]types.Transaction, error) {
	if amount == 0 {
		return []types.Transaction{}, nil
	}
	txIdKey := make([]byte, 8)
	reader := bytes.NewReader(nil)
	stream := rlp.NewStream(reader, 0)
	txs := make([]types.Transaction, amount)
	binary.BigEndian.PutUint64(txIdKey, baseTxId)
	i := uint32(0)

	if err := db.ForAmount(kv.EthTx, txIdKey, amount, func(k, v []byte) error {
		var decodeErr error
		reader.Reset(v)
		stream.Reset(reader, 0)
		if txs[i], decodeErr = types.DecodeTransaction(stream); decodeErr != nil {
			return decodeErr
		}
		i++
		return nil
	}); err != nil {
		return nil, err
	}
	txs = txs[:i] // user may request big "amount", but db can return small "amount". Return as much as we found.
	return txs, nil
}

func ReadBodyWithTransactions(db kv.Getter, hash common.Hash, number uint64) *types.Body {
	body, baseTxId, txAmount := ReadBody(db, hash, number)
	if body == nil {
		return nil
	}
	var err error
	body.Transactions, err = CanonicalTransactions(db, baseTxId, txAmount)
	if err != nil {
		log.Error("failed ReadTransaction", "hash", hash, "block", number, "err", err)
		return nil
	}
	return body
}
