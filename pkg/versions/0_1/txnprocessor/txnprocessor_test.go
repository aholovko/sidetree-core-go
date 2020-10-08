/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txnprocessor

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trustbloc/sidetree-core-go/pkg/api/batch"
	"github.com/trustbloc/sidetree-core-go/pkg/api/txn"
)

const anchorString = "1.anchorAddress"

func TestTxnProcessor_Process(t *testing.T) {
	t.Run("test error from txn operations provider", func(t *testing.T) {
		errExpected := fmt.Errorf("txn operations provider error")

		opp := &mockTxnOpsProvider{
			err: errExpected,
		}

		providers := &Providers{
			OpStore:                   &mockOperationStore{},
			OperationProtocolProvider: opp,
		}

		p := New(providers)
		err := p.Process(txn.SidetreeTxn{})
		require.Error(t, err)
		require.Contains(t, err.Error(), errExpected.Error())
	})
}

func TestProcessTxnOperations(t *testing.T) {
	t.Run("test error from operationStore Put", func(t *testing.T) {
		providers := &Providers{
			OpStore: &mockOperationStore{putFunc: func(ops []*batch.AnchoredOperation) error {
				return fmt.Errorf("put error")
			}},
		}

		p := New(providers)
		err := p.processTxnOperations([]*batch.AnchoredOperation{{UniqueSuffix: "abc"}}, txn.SidetreeTxn{AnchorString: anchorString})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to store operation from anchor string")
	})

	t.Run("test success", func(t *testing.T) {
		providers := &Providers{
			OperationProtocolProvider: &mockTxnOpsProvider{},
			OpStore:                   &mockOperationStore{},
		}

		p := New(providers)
		batchOps, err := p.OperationProtocolProvider.GetTxnOperations(&txn.SidetreeTxn{AnchorString: anchorString})
		require.NoError(t, err)

		err = p.processTxnOperations(batchOps, txn.SidetreeTxn{AnchorString: anchorString})
		require.NoError(t, err)
	})

	t.Run("success - multiple operations with same suffix in transaction operations", func(t *testing.T) {
		providers := &Providers{
			OperationProtocolProvider: &mockTxnOpsProvider{},
			OpStore:                   &mockOperationStore{},
		}

		p := New(providers)
		batchOps, err := p.OperationProtocolProvider.GetTxnOperations(&txn.SidetreeTxn{AnchorString: anchorString})
		require.NoError(t, err)

		// add same operations again to create scenario where batch has multiple operations with same suffix
		// only first operation will be processed, subsequent operations will be discarded
		batchOps = append(batchOps, batchOps...)

		err = p.processTxnOperations(batchOps, txn.SidetreeTxn{AnchorString: anchorString})
		require.NoError(t, err)
	})
}

func TestUpdateOperation(t *testing.T) {
	t.Run("test success", func(t *testing.T) {
		updatedOps := updateAnchoredOperation(&batch.AnchoredOperation{UniqueSuffix: "abc"},
			txn.SidetreeTxn{TransactionTime: 20, TransactionNumber: 2})
		require.Equal(t, uint64(20), updatedOps.TransactionTime)
		require.Equal(t, uint64(2), updatedOps.TransactionNumber)
	})
}

type mockOperationStore struct {
	putFunc func(ops []*batch.AnchoredOperation) error
	getFunc func(suffix string) ([]*batch.AnchoredOperation, error)
}

func (m *mockOperationStore) Put(ops []*batch.AnchoredOperation) error {
	if m.putFunc != nil {
		return m.putFunc(ops)
	}

	return nil
}

func (m *mockOperationStore) Get(suffix string) ([]*batch.AnchoredOperation, error) {
	if m.getFunc != nil {
		return m.getFunc(suffix)
	}

	return nil, nil
}

type mockTxnOpsProvider struct {
	err error
}

func (m *mockTxnOpsProvider) GetTxnOperations(txn *txn.SidetreeTxn) ([]*batch.AnchoredOperation, error) {
	if m.err != nil {
		return nil, m.err
	}

	op := &batch.AnchoredOperation{
		UniqueSuffix: "abc",
	}

	return []*batch.AnchoredOperation{op}, nil
}
