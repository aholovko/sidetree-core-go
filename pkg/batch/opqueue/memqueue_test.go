/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package opqueue

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trustbloc/sidetree-core-go/pkg/api/batch"
)

var (
	op1 = &batch.OperationInfo{Namespace: "ns", UniqueSuffix: "op1", Data: []byte("op1")}
	op2 = &batch.OperationInfo{Namespace: "ns", UniqueSuffix: "op2", Data: []byte("op2")}
	op3 = &batch.OperationInfo{Namespace: "ns", UniqueSuffix: "op3", Data: []byte("op3")}
)

func TestMemQueue(t *testing.T) {
	q := &MemQueue{}
	require.Zero(t, q.Len())

	ops, err := q.Peek(1)
	require.NoError(t, err)
	require.Empty(t, ops)

	l, err := q.Add(op1, 10)
	require.NoError(t, err)
	require.Equal(t, uint(1), l)
	require.Equal(t, uint(1), q.Len())

	l, err = q.Add(op2, 10)
	require.NoError(t, err)
	require.Equal(t, uint(2), l)
	require.Equal(t, uint(2), q.Len())

	l, err = q.Add(op3, 10)
	require.NoError(t, err)
	require.Equal(t, uint(3), l)
	require.Equal(t, uint(3), q.Len())

	ops, err = q.Peek(1)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, *op1, ops[0].OperationInfo)

	ops, err = q.Peek(4)
	require.NoError(t, err)
	require.Len(t, ops, 3)
	require.Equal(t, *op1, ops[0].OperationInfo)
	require.Equal(t, *op2, ops[1].OperationInfo)
	require.Equal(t, *op3, ops[2].OperationInfo)

	n, l, err := q.Remove(1)
	require.NoError(t, err)
	require.Equal(t, uint(1), n)
	require.Equal(t, uint(2), l)

	ops, err = q.Peek(1)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, *op2, ops[0].OperationInfo)

	n, l, err = q.Remove(5)
	require.NoError(t, err)
	require.Equal(t, uint(2), n)
	require.Zero(t, l)
}
