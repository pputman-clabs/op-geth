// Code generated by rlpgen. DO NOT EDIT.

package types

import "github.com/ethereum/go-ethereum/common"
import "github.com/ethereum/go-ethereum/rlp"
import "io"

func (obj *BeforeGingerbreadHeader) EncodeRLP(_w io.Writer) error {
	w := rlp.NewEncoderBuffer(_w)
	_tmp0 := w.List()
	w.WriteBytes(obj.ParentHash[:])
	w.WriteBytes(obj.Coinbase[:])
	w.WriteBytes(obj.Root[:])
	w.WriteBytes(obj.TxHash[:])
	w.WriteBytes(obj.ReceiptHash[:])
	w.WriteBytes(obj.Bloom[:])
	if obj.Number == nil {
		w.Write(rlp.EmptyString)
	} else {
		if obj.Number.Sign() == -1 {
			return rlp.ErrNegativeBigInt
		}
		w.WriteBigInt(obj.Number)
	}
	w.WriteUint64(obj.GasUsed)
	w.WriteUint64(obj.Time)
	w.WriteBytes(obj.Extra)
	w.ListEnd(_tmp0)
	return w.Flush()
}

func (obj *BeforeGingerbreadHeader) DecodeRLP(dec *rlp.Stream) error {
	var _tmp0 BeforeGingerbreadHeader
	{
		if _, err := dec.List(); err != nil {
			return err
		}
		// ParentHash:
		var _tmp1 common.Hash
		if err := dec.ReadBytes(_tmp1[:]); err != nil {
			return err
		}
		_tmp0.ParentHash = _tmp1
		// Coinbase:
		var _tmp2 common.Address
		if err := dec.ReadBytes(_tmp2[:]); err != nil {
			return err
		}
		_tmp0.Coinbase = _tmp2
		// Root:
		var _tmp3 common.Hash
		if err := dec.ReadBytes(_tmp3[:]); err != nil {
			return err
		}
		_tmp0.Root = _tmp3
		// TxHash:
		var _tmp4 common.Hash
		if err := dec.ReadBytes(_tmp4[:]); err != nil {
			return err
		}
		_tmp0.TxHash = _tmp4
		// ReceiptHash:
		var _tmp5 common.Hash
		if err := dec.ReadBytes(_tmp5[:]); err != nil {
			return err
		}
		_tmp0.ReceiptHash = _tmp5
		// Bloom:
		var _tmp6 Bloom
		if err := dec.ReadBytes(_tmp6[:]); err != nil {
			return err
		}
		_tmp0.Bloom = _tmp6
		// Number:
		_tmp7, err := dec.BigInt()
		if err != nil {
			return err
		}
		_tmp0.Number = _tmp7
		// GasUsed:
		_tmp8, err := dec.Uint64()
		if err != nil {
			return err
		}
		_tmp0.GasUsed = _tmp8
		// Time:
		_tmp9, err := dec.Uint64()
		if err != nil {
			return err
		}
		_tmp0.Time = _tmp9
		// Extra:
		_tmp10, err := dec.Bytes()
		if err != nil {
			return err
		}
		_tmp0.Extra = _tmp10
		if err := dec.ListEnd(); err != nil {
			return err
		}
	}
	*obj = _tmp0
	return nil
}
