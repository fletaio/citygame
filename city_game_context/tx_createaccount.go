package citygame

import (
	"bytes"
	"io"
	"strconv"

	"git.fleta.io/fleta/extension/utxo_tx"

	"git.fleta.io/fleta/core/amount"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/common/hash"
	"git.fleta.io/fleta/common/util"
	"git.fleta.io/fleta/core/data"
	"git.fleta.io/fleta/core/transaction"
)

// prefix values
var (
	PrefixKeyHash = []byte("k:")
	PrefixUserID  = []byte("i:")
	PrefixReward  = []byte("r:")
)

var allowedPublicHashMap = map[uint64]common.PublicHash{}

// RegisterAllowedPublicHash TODO
func RegisterAllowedPublicHash(ChainCoord *common.Coordinate, pubhash common.PublicHash) {
	allowedPublicHashMap[ChainCoord.ID()] = pubhash
}

// UnregisterAllowedPublicHash  TODO
func UnregisterAllowedPublicHash(ChainCoord *common.Coordinate) {
	delete(allowedPublicHashMap, ChainCoord.ID())
}

func init() {
	data.RegisterTransaction("fletacity.CreateAccount", func(coord *common.Coordinate, t transaction.Type) transaction.Transaction {
		return &CreateAccountTx{
			Base: utxo_tx.Base{
				Base: transaction.Base{
					ChainCoord_: coord,
					Type_:       t,
				},
				Vin: []*transaction.TxIn{},
			},
		}
	}, func(loader data.Loader, t transaction.Transaction, signers []common.PublicHash) error {
		tx := t.(*CreateAccountTx)
		if len(tx.Vin) != 1 {
			return ErrInvalidTxInCount
		}
		if len(signers) != 1 {
			return ErrInvalidSignerCount
		}

		if utxo, err := loader.UTXO(tx.Vin[0].ID()); err != nil {
			return err
		} else {
			if !utxo.PublicHash.Equal(signers[0]) {
				return ErrInvalidTransactionSignature
			}
			pubhash := allowedPublicHashMap[loader.ChainCoord().ID()]
			if !utxo.PublicHash.Equal(pubhash) {
				return ErrNotAllowed
			}
		}
		return nil
	}, func(ctx *data.Context, Fee *amount.Amount, t transaction.Transaction, coord *common.Coordinate) (interface{}, error) {
		tx := t.(*CreateAccountTx)
		sn := ctx.Snapshot()
		defer ctx.Revert(sn)

		utxo, err := ctx.UTXO(tx.Vin[0].ID())
		if err != nil {
			return nil, err
		}

		if err := ctx.DeleteUTXO(utxo.ID()); err != nil {
			return nil, err
		}

		func() {
			sn := ctx.Snapshot()
			defer ctx.Revert(sn)

			chainCoord := ctx.ChainCoord()
			addr := common.NewAddress(coord, chainCoord, 0)
			if is, err := ctx.IsExistAccount(addr); err != nil {
				//log.Println(err)
				return
			} else if is {
				//log.Println(ErrExistAddress)
				return
			}

			KeyHashID := append(PrefixKeyHash, tx.KeyHash[:]...)
			UserIDHashID := append(PrefixUserID, []byte(tx.UserID)...)
			RewardHashID := append(PrefixReward, []byte(tx.Reward)...)

			var rootAddress common.Address
			if bs := ctx.AccountData(rootAddress, KeyHashID); len(bs) > 0 {
				//log.Println(ErrExistKeyHash)
				return
			}
			if bs := ctx.AccountData(rootAddress, UserIDHashID); len(bs) > 0 {
				//log.Println(ErrExistKeyHash)
				return
			}
			if bs := ctx.AccountData(rootAddress, RewardHashID); len(bs) > 0 {
				//log.Println(ErrExistKeyHash)
				return
			}

			a, err := ctx.Accounter().NewByTypeName("fletacity.Account")
			if err != nil {
				//log.Println(err)
				return
			}
			acc := a.(*Account)
			acc.Address_ = addr
			acc.KeyHash = tx.KeyHash
			ctx.CreateAccount(acc)

			gd := NewGameData(ctx.TargetHeight())
			var buffer bytes.Buffer
			if _, err := gd.WriteTo(&buffer); err != nil {
				//log.Println(err)
				return
			}
			ctx.SetAccountData(addr, []byte("game"), buffer.Bytes())

			ctx.SetAccountData(rootAddress, KeyHashID, addr[:])
			ctx.SetAccountData(rootAddress, UserIDHashID, addr[:])
			ctx.SetAccountData(rootAddress, RewardHashID, addr[:])

			for i := 0; i < GameAccountChannelSize; i++ {
				id := transaction.MarshalID(coord.Height, coord.Index, uint16(i+1))
				ctx.CreateUTXO(id, &transaction.TxOut{
					Amount:     amount.NewCoinAmount(0, 0),
					PublicHash: tx.KeyHash,
				})
				ctx.SetAccountData(addr, []byte("utxo"+strconv.Itoa(i)), util.Uint64ToBytes(id))
			}

			ctx.Commit(sn)
		}()

		var rootAddress common.Address
		for i := 0; i < CreateAccountChannelSize; i++ {
			did := []byte("utxo" + strconv.Itoa(i))
			oldid := util.BytesToUint64(ctx.AccountData(rootAddress, did))
			if oldid == tx.Vin[0].ID() {
				id := transaction.MarshalID(coord.Height, coord.Index, 0)
				ctx.CreateUTXO(id, &transaction.TxOut{
					Amount:     amount.NewCoinAmount(0, 0),
					PublicHash: utxo.PublicHash,
				})
				ctx.SetAccountData(rootAddress, did, util.Uint64ToBytes(id))
				break
			}
		}

		ctx.Commit(sn)
		return nil, nil
	})
}

// CreateAccountTx is a fleta.CreateAccountTx
// It is used to make a single account
type CreateAccountTx struct {
	utxo_tx.Base
	KeyHash common.PublicHash
	UserID  string `json:"user_id"`
	Reward  string `json:"reward"`
}

// Hash returns the hash value of it
func (tx *CreateAccountTx) Hash() hash.Hash256 {
	return hash.DoubleHashByWriterTo(tx)
}

// WriteTo is a serialization function
func (tx *CreateAccountTx) WriteTo(w io.Writer) (int64, error) {
	var wrote int64
	if n, err := tx.Base.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := tx.KeyHash.WriteTo(w); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteString(w, tx.UserID); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	if n, err := util.WriteString(w, tx.Reward); err != nil {
		return wrote, err
	} else {
		wrote += n
	}
	return wrote, nil
}

// ReadFrom is a deserialization function
func (tx *CreateAccountTx) ReadFrom(r io.Reader) (int64, error) {
	var read int64
	if n, err := tx.Base.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}
	if n, err := tx.KeyHash.ReadFrom(r); err != nil {
		return read, err
	} else {
		read += n
	}
	if v, n, err := util.ReadString(r); err != nil {
		return read, err
	} else {
		read += n
		tx.UserID = v
	}
	if v, n, err := util.ReadString(r); err != nil {
		return read, err
	} else {
		read += n
		tx.Reward = v
	}
	return read, nil
}