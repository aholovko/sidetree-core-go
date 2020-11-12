/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txnprovider

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trustbloc/sidetree-core-go/pkg/api/operation"
	"github.com/trustbloc/sidetree-core-go/pkg/api/protocol"
	"github.com/trustbloc/sidetree-core-go/pkg/api/txn"
	"github.com/trustbloc/sidetree-core-go/pkg/compression"
	"github.com/trustbloc/sidetree-core-go/pkg/mocks"
	"github.com/trustbloc/sidetree-core-go/pkg/patch"
	"github.com/trustbloc/sidetree-core-go/pkg/versions/0_1/doccomposer"
	"github.com/trustbloc/sidetree-core-go/pkg/versions/0_1/model"
	"github.com/trustbloc/sidetree-core-go/pkg/versions/0_1/operationparser"
	"github.com/trustbloc/sidetree-core-go/pkg/versions/0_1/txnprovider/models"
)

const (
	compressionAlgorithm = "GZIP"
	maxFileSize          = 2000 // in bytes
)

func TestNewOperationProvider(t *testing.T) {
	pc := mocks.NewMockProtocolClient()

	handler := NewOperationProvider(
		pc.Protocol,
		operationparser.New(pc.Protocol),
		mocks.NewMockCasClient(nil),
		compression.New(compression.WithDefaultAlgorithms()))

	require.NotNil(t, handler)
}

func TestHandler_GetTxnOperations(t *testing.T) {
	const createOpsNum = 2
	const updateOpsNum = 3
	const deactivateOpsNum = 2
	const recoverOpsNum = 2

	pc := mocks.NewMockProtocolClient()
	parser := operationparser.New(pc.Protocol)
	cp := compression.New(compression.WithDefaultAlgorithms())

	t.Run("success", func(t *testing.T) {
		cas := mocks.NewMockCasClient(nil)
		handler := NewOperationHandler(pc.Protocol, cas, cp, operationparser.New(pc.Protocol))

		ops := getTestOperations(createOpsNum, updateOpsNum, deactivateOpsNum, recoverOpsNum)

		anchorString, err := handler.PrepareTxnFiles(ops)
		require.NoError(t, err)
		require.NotEmpty(t, anchorString)

		provider := NewOperationProvider(pc.Protocol, parser, cas, cp)

		txnOps, err := provider.GetTxnOperations(&txn.SidetreeTxn{
			Namespace:         defaultNS,
			AnchorString:      anchorString,
			TransactionNumber: 1,
			TransactionTime:   1,
		})

		require.NoError(t, err)
		require.Equal(t, createOpsNum+updateOpsNum+deactivateOpsNum+recoverOpsNum, len(txnOps))
	})

	t.Run("error - number of operations doesn't match", func(t *testing.T) {
		cas := mocks.NewMockCasClient(nil)
		handler := NewOperationHandler(pc.Protocol, cas, cp, operationparser.New(pc.Protocol))

		ops := getTestOperations(createOpsNum, updateOpsNum, deactivateOpsNum, recoverOpsNum)

		// anchor string has 9 operations "9.anchorAddress"
		anchorString, err := handler.PrepareTxnFiles(ops)
		require.NoError(t, err)
		require.NotEmpty(t, anchorString)

		// update number of operations in anchor string from 9 to 7
		ad, err := ParseAnchorData(anchorString)
		require.NoError(t, err)
		ad.NumberOfOperations = 7
		anchorString = ad.GetAnchorString()

		provider := NewOperationProvider(mocks.NewMockProtocolClient().Protocol, operationparser.New(pc.Protocol), cas, cp)

		txnOps, err := provider.GetTxnOperations(&txn.SidetreeTxn{
			Namespace:         defaultNS,
			AnchorString:      anchorString,
			TransactionNumber: 1,
			TransactionTime:   1,
		})

		require.Error(t, err)
		require.Nil(t, txnOps)
		require.Contains(t, err.Error(), "number of txn ops[9] doesn't match anchor string num of ops[7]")
	})

	t.Run("error - read from CAS error", func(t *testing.T) {
		protocolClient := mocks.NewMockProtocolClient()
		handler := NewOperationProvider(protocolClient.Protocol, operationparser.New(protocolClient.Protocol), mocks.NewMockCasClient(errors.New("CAS error")), cp)

		txnOps, err := handler.GetTxnOperations(&txn.SidetreeTxn{
			Namespace:         defaultNS,
			AnchorString:      "1" + delimiter + "anchor",
			TransactionNumber: 1,
			TransactionTime:   1,
		})

		require.Error(t, err)
		require.Nil(t, txnOps)
		require.Contains(t, err.Error(), "error reading core index file: retrieve CAS content at uri[anchor]: CAS error")
	})

	t.Run("error - parse anchor operations error", func(t *testing.T) {
		cas := mocks.NewMockCasClient(nil)
		handler := NewOperationHandler(pc.Protocol, cas, cp, operationparser.New(pc.Protocol))

		ops := getTestOperations(createOpsNum, updateOpsNum, deactivateOpsNum, recoverOpsNum)

		anchorString, err := handler.PrepareTxnFiles(ops)
		require.NoError(t, err)
		require.NotEmpty(t, anchorString)

		invalid := mocks.NewMockProtocolClient().Protocol
		invalid.MultihashAlgorithm = 55

		provider := NewOperationProvider(invalid, operationparser.New(invalid), cas, cp)

		txnOps, err := provider.GetTxnOperations(&txn.SidetreeTxn{
			Namespace:         mocks.DefaultNS,
			AnchorString:      anchorString,
			TransactionNumber: 1,
			TransactionTime:   1,
		})

		require.Error(t, err)
		require.Nil(t, txnOps)
		require.Contains(t, err.Error(), "parse anchor operations: algorithm not supported")
	})

	t.Run("error - parse anchor data error", func(t *testing.T) {
		p := mocks.NewMockProtocolClient().Protocol
		provider := NewOperationProvider(p, operationparser.New(p), mocks.NewMockCasClient(nil), cp)

		txnOps, err := provider.GetTxnOperations(&txn.SidetreeTxn{
			AnchorString:      "abc.anchor",
			TransactionNumber: 1,
			TransactionTime:   1,
		})

		require.Error(t, err)
		require.Nil(t, txnOps)
		require.Contains(t, err.Error(), "parse anchor data[abc.anchor] failed")
	})

	t.Run("success - deactivate only", func(t *testing.T) {
		const deactivateOpsNum = 2

		var ops []*operation.QueuedOperation
		ops = append(ops, generateOperations(deactivateOpsNum, operation.TypeDeactivate)...)

		cas := mocks.NewMockCasClient(nil)
		handler := NewOperationHandler(pc.Protocol, cas, cp, operationparser.New(pc.Protocol))

		anchorString, err := handler.PrepareTxnFiles(ops)
		require.NoError(t, err)
		require.NotEmpty(t, anchorString)

		p := mocks.NewMockProtocolClient().Protocol
		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		txnOps, err := provider.GetTxnOperations(&txn.SidetreeTxn{
			Namespace:         defaultNS,
			AnchorString:      anchorString,
			TransactionNumber: 1,
			TransactionTime:   1,
		})

		require.NoError(t, err)
		require.Equal(t, deactivateOpsNum, len(txnOps))
	})
}

func TestHandler_GetCoreIndexFile(t *testing.T) {
	cp := compression.New(compression.WithDefaultAlgorithms())
	p := protocol.Protocol{MaxCoreIndexFileSize: maxFileSize, CompressionAlgorithm: compressionAlgorithm}

	cas := mocks.NewMockCasClient(nil)
	content, err := cp.Compress(compressionAlgorithm, []byte("{}"))
	require.NoError(t, err)
	address, err := cas.Write(content)
	require.NoError(t, err)

	parser := operationparser.New(p)

	t.Run("success", func(t *testing.T) {
		provider := NewOperationProvider(p, parser, cas, cp)

		file, err := provider.getCoreIndexFile(address)
		require.NoError(t, err)
		require.NotNil(t, file)
	})

	t.Run("error - core index file exceeds maximum size", func(t *testing.T) {
		provider := NewOperationProvider(protocol.Protocol{MaxCoreIndexFileSize: 15, CompressionAlgorithm: compressionAlgorithm}, parser, cas, cp)

		file, err := provider.getCoreIndexFile(address)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "exceeded maximum size 15")
	})

	t.Run("error - parse core index file error (invalid JSON)", func(t *testing.T) {
		cas := mocks.NewMockCasClient(nil)
		content, err := cp.Compress(compressionAlgorithm, []byte("invalid"))
		require.NoError(t, err)
		address, err := cas.Write(content)

		provider := NewOperationProvider(p, parser, cas, cp)
		file, err := provider.getCoreIndexFile(address)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "failed to parse content for core index file")
	})
}

func TestHandler_GetProvisionalIndexFile(t *testing.T) {
	cp := compression.New(compression.WithDefaultAlgorithms())
	p := protocol.Protocol{MaxProvisionalIndexFileSize: maxFileSize, CompressionAlgorithm: compressionAlgorithm}

	cas := mocks.NewMockCasClient(nil)
	content, err := cp.Compress(compressionAlgorithm, []byte("{}"))
	require.NoError(t, err)
	address, err := cas.Write(content)
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.getProvisionalIndexFile(address)
		require.NoError(t, err)
		require.NotNil(t, file)
	})

	t.Run("error - provisional index file exceeds maximum size", func(t *testing.T) {
		lowMaxFileSize := protocol.Protocol{MaxProvisionalIndexFileSize: 5, CompressionAlgorithm: compressionAlgorithm}
		parser := operationparser.New(lowMaxFileSize)
		provider := NewOperationProvider(lowMaxFileSize, parser, cas, cp)

		file, err := provider.getProvisionalIndexFile(address)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "exceeded maximum size 5")
	})

	t.Run("error - parse core index file error (invalid JSON)", func(t *testing.T) {
		cas := mocks.NewMockCasClient(nil)
		content, err := cp.Compress(compressionAlgorithm, []byte("invalid"))
		require.NoError(t, err)
		address, err := cas.Write(content)

		parser := operationparser.New(p)
		provider := NewOperationProvider(p, parser, cas, cp)
		file, err := provider.getProvisionalIndexFile(address)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "failed to parse content for provisional index file")
	})
}

func TestHandler_GetChunkFile(t *testing.T) {
	cp := compression.New(compression.WithDefaultAlgorithms())
	p := protocol.Protocol{MaxChunkFileSize: maxFileSize, CompressionAlgorithm: compressionAlgorithm}

	cas := mocks.NewMockCasClient(nil)
	content, err := cp.Compress(compressionAlgorithm, []byte("{}"))
	require.NoError(t, err)
	address, err := cas.Write(content)
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.getChunkFile(address)
		require.NoError(t, err)
		require.NotNil(t, file)
	})

	t.Run("error - chunk file exceeds maximum size", func(t *testing.T) {
		lowMaxFileSize := protocol.Protocol{MaxChunkFileSize: 10, CompressionAlgorithm: compressionAlgorithm}
		provider := NewOperationProvider(lowMaxFileSize, operationparser.New(p), cas, cp)

		file, err := provider.getChunkFile(address)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "exceeded maximum size 10")
	})

	t.Run("error - parse chunk file error (invalid JSON)", func(t *testing.T) {
		content, err := cp.Compress(compressionAlgorithm, []byte("invalid"))
		require.NoError(t, err)
		address, err := cas.Write(content)

		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)
		file, err := provider.getChunkFile(address)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "failed to parse content for chunk file")
	})
}

func TestHandler_readFromCAS(t *testing.T) {
	cp := compression.New(compression.WithDefaultAlgorithms())
	p := protocol.Protocol{MaxChunkFileSize: maxFileSize, CompressionAlgorithm: compressionAlgorithm}

	cas := mocks.NewMockCasClient(nil)
	content, err := cp.Compress(compressionAlgorithm, []byte("{}"))
	require.NoError(t, err)
	address, err := cas.Write(content)
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.readFromCAS(address, compressionAlgorithm, maxFileSize)
		require.NoError(t, err)
		require.NotNil(t, file)
	})

	t.Run("error - read from CAS error", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), mocks.NewMockCasClient(errors.New("CAS error")), cp)

		file, err := provider.getChunkFile("address")
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), " retrieve CAS content at uri[address]: CAS error")
	})

	t.Run("error - content exceeds maximum size", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.readFromCAS(address, compressionAlgorithm, 20)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "exceeded maximum size 20")
	})

	t.Run("error - decompression error", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.readFromCAS(address, "alg", maxFileSize)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "compression algorithm 'alg' not supported")
	})
}

func TestHandler_GetCorePoofFile(t *testing.T) {
	cp := compression.New(compression.WithDefaultAlgorithms())
	p := protocol.Protocol{MaxProofFileSize: maxFileSize, CompressionAlgorithm: compressionAlgorithm}

	cas := mocks.NewMockCasClient(nil)
	content, err := cp.Compress(compressionAlgorithm, []byte("{}"))
	require.NoError(t, err)
	uri, err := cas.Write(content)
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.getCoreProofFile(uri)
		require.NoError(t, err)
		require.NotNil(t, file)
	})

	t.Run("error - core proof file exceeds maximum size", func(t *testing.T) {
		lowMaxFileSize := protocol.Protocol{MaxProofFileSize: 10, CompressionAlgorithm: compressionAlgorithm}
		provider := NewOperationProvider(lowMaxFileSize, operationparser.New(p), cas, cp)

		file, err := provider.getCoreProofFile(uri)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "exceeded maximum size 10")
	})

	t.Run("error - parse core proof file error (invalid JSON)", func(t *testing.T) {
		content, err := cp.Compress(compressionAlgorithm, []byte("invalid"))
		require.NoError(t, err)
		address, err := cas.Write(content)

		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)
		file, err := provider.getCoreProofFile(address)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "failed to parse content for core proof file")
	})
}

func TestHandler_GetProvisionalPoofFile(t *testing.T) {
	cp := compression.New(compression.WithDefaultAlgorithms())
	p := protocol.Protocol{MaxProofFileSize: maxFileSize, CompressionAlgorithm: compressionAlgorithm}

	cas := mocks.NewMockCasClient(nil)
	content, err := cp.Compress(compressionAlgorithm, []byte("{}"))
	require.NoError(t, err)
	uri, err := cas.Write(content)
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.getProvisionalProofFile(uri)
		require.NoError(t, err)
		require.NotNil(t, file)
	})

	t.Run("error - core provisional file exceeds maximum size", func(t *testing.T) {
		lowMaxFileSize := protocol.Protocol{MaxProofFileSize: 10, CompressionAlgorithm: compressionAlgorithm}
		provider := NewOperationProvider(lowMaxFileSize, operationparser.New(p), cas, cp)

		file, err := provider.getProvisionalProofFile(uri)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "exceeded maximum size 10")
	})

	t.Run("error - parse provisional proof file error (invalid JSON)", func(t *testing.T) {
		content, err := cp.Compress(compressionAlgorithm, []byte("invalid"))
		require.NoError(t, err)
		address, err := cas.Write(content)

		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)
		file, err := provider.getProvisionalProofFile(address)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "failed to parse content for provisional proof file")
	})
}

func TestHandler_GetBatchFiles(t *testing.T) {
	cp := compression.New(compression.WithDefaultAlgorithms())

	cas := mocks.NewMockCasClient(nil)
	content, err := cp.Compress(compressionAlgorithm, []byte("{}"))
	require.NoError(t, err)
	uri, err := cas.Write(content)
	require.NoError(t, err)

	provisionalIndexFile, err := cp.Compress(compressionAlgorithm, []byte(fmt.Sprintf(`{"provisionalProofFileUri":"%s","chunks":[{"chunkFileUri":"%s"}]}`, uri, uri)))
	require.NoError(t, err)
	mapURI, err := cas.Write(provisionalIndexFile)
	require.NoError(t, err)

	af := &models.CoreIndexFile{
		ProvisionalIndexFileURI: mapURI,
		CoreProofFileURI:        uri,
	}

	t.Run("success", func(t *testing.T) {
		p := newMockProtocolClient().Protocol
		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.getBatchFiles(af)
		require.NoError(t, err)
		require.NotNil(t, file)
	})

	t.Run("error - retrieve provisional index file", func(t *testing.T) {
		p := newMockProtocolClient().Protocol
		p.MaxProvisionalIndexFileSize = 10

		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.getBatchFiles(af)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "exceeded maximum size 10")
	})

	t.Run("error - retrieve core proof file", func(t *testing.T) {
		p := newMockProtocolClient().Protocol
		p.MaxProofFileSize = 7

		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.getBatchFiles(af)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "exceeded maximum size 7")
	})

	t.Run("error - retrieve provisional proof file", func(t *testing.T) {
		p := newMockProtocolClient().Protocol

		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		content, err := cp.Compress(compressionAlgorithm, []byte("invalid"))
		ppfURI, err := cas.Write(content)
		require.NoError(t, err)

		provisionalIndexFile, err := cp.Compress(compressionAlgorithm, []byte(fmt.Sprintf(`{"provisionalProofFileUri":"%s","chunks":[{"chunkFileUri":"%s"}]}`, ppfURI, uri)))
		require.NoError(t, err)
		provisionalIndexURI, err := cas.Write(provisionalIndexFile)
		require.NoError(t, err)

		af2 := &models.CoreIndexFile{
			ProvisionalIndexFileURI: provisionalIndexURI,
			CoreProofFileURI:        uri,
		}

		file, err := provider.getBatchFiles(af2)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "failed to unmarshal provisional proof file: invalid character")
	})

	t.Run("error - provisional index file is missing chunk file URI", func(t *testing.T) {
		p := newMockProtocolClient().Protocol

		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		af2 := &models.CoreIndexFile{
			ProvisionalIndexFileURI: uri,
			CoreProofFileURI:        uri,
		}

		file, err := provider.getBatchFiles(af2)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "provisional index file is missing chunk file URI")
	})

	t.Run("error - retrieve chunk file", func(t *testing.T) {
		p := newMockProtocolClient().Protocol
		p.MaxChunkFileSize = 10

		provider := NewOperationProvider(p, operationparser.New(p), cas, cp)

		file, err := provider.getBatchFiles(af)
		require.Error(t, err)
		require.Nil(t, file)
		require.Contains(t, err.Error(), "exceeded maximum size 10")
	})
}

func TestHandler_assembleBatchOperations(t *testing.T) {
	p := newMockProtocolClient().Protocol

	t.Run("success", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), nil, nil)

		createOp, err := generateOperation(1, operation.TypeCreate)
		require.NoError(t, err)

		updateOp, err := generateOperation(2, operation.TypeUpdate)
		require.NoError(t, err)

		deactivateOp, err := generateOperation(3, operation.TypeDeactivate)
		require.NoError(t, err)

		cif := &models.CoreIndexFile{
			ProvisionalIndexFileURI: "hash",
			Operations: models.CoreOperations{
				Create:     []models.CreateOperation{{SuffixData: createOp.SuffixData}},
				Deactivate: []models.SignedOperation{{DidSuffix: deactivateOp.UniqueSuffix, SignedData: deactivateOp.SignedData}},
			},
		}

		pif := &models.ProvisionalIndexFile{
			Chunks: []models.Chunk{},
			Operations: models.ProvisionalOperations{
				Update: []models.SignedOperation{{DidSuffix: updateOp.UniqueSuffix, SignedData: updateOp.SignedData}},
			},
		}

		cf := &models.ChunkFile{Deltas: []*model.DeltaModel{createOp.Delta, updateOp.Delta}}

		batchFiles := &batchFiles{
			CoreIndex:        cif,
			ProvisionalIndex: pif,
			Chunk:            cf,
		}

		file, err := provider.assembleBatchOperations(batchFiles, &txn.SidetreeTxn{Namespace: defaultNS})
		require.NoError(t, err)
		require.NotNil(t, file)
	})

	t.Run("error - core/provisional index, chunk file operation number mismatch", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), nil, nil)

		createOp, err := generateOperation(1, operation.TypeCreate)
		require.NoError(t, err)

		updateOp, err := generateOperation(2, operation.TypeUpdate)
		require.NoError(t, err)

		deactivateOp, err := generateOperation(3, operation.TypeDeactivate)
		require.NoError(t, err)

		cif := &models.CoreIndexFile{
			ProvisionalIndexFileURI: "hash",
			Operations: models.CoreOperations{
				Create: []models.CreateOperation{{SuffixData: createOp.SuffixData}},
				Deactivate: []models.SignedOperation{
					{DidSuffix: deactivateOp.UniqueSuffix, SignedData: deactivateOp.SignedData},
				},
			},
		}

		pif := &models.ProvisionalIndexFile{
			Chunks: []models.Chunk{},
			Operations: models.ProvisionalOperations{
				Update: []models.SignedOperation{{DidSuffix: updateOp.UniqueSuffix, SignedData: updateOp.SignedData}},
			},
		}

		// don't add update operation delta to chunk file in order to cause error
		cf := &models.ChunkFile{Deltas: []*model.DeltaModel{createOp.Delta}}

		batchFiles := &batchFiles{
			CoreIndex:        cif,
			ProvisionalIndex: pif,
			Chunk:            cf,
		}

		anchoredOps, err := provider.assembleBatchOperations(batchFiles, &txn.SidetreeTxn{Namespace: defaultNS})
		require.Error(t, err)
		require.Nil(t, anchoredOps)
		require.Contains(t, err.Error(),
			"number of create+recover+update operations[2] doesn't match number of deltas[1]")
	})

	t.Run("error - duplicate operations found in core/provisional index files", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), nil, nil)

		createOp, err := generateOperation(1, operation.TypeCreate)
		require.NoError(t, err)

		updateOp, err := generateOperation(2, operation.TypeUpdate)
		require.NoError(t, err)

		deactivateOp, err := generateOperation(3, operation.TypeDeactivate)
		require.NoError(t, err)

		cif := &models.CoreIndexFile{
			ProvisionalIndexFileURI: "hash",
			Operations: models.CoreOperations{
				Create: []models.CreateOperation{{SuffixData: createOp.SuffixData}},
				Deactivate: []models.SignedOperation{
					{DidSuffix: deactivateOp.UniqueSuffix, SignedData: deactivateOp.SignedData},
					{DidSuffix: deactivateOp.UniqueSuffix, SignedData: deactivateOp.SignedData},
				},
			},
		}

		pif := &models.ProvisionalIndexFile{
			Chunks: []models.Chunk{},
			Operations: models.ProvisionalOperations{
				Update: []models.SignedOperation{
					{DidSuffix: updateOp.UniqueSuffix, SignedData: updateOp.SignedData},
					{DidSuffix: updateOp.UniqueSuffix, SignedData: updateOp.SignedData},
				},
			},
		}

		cf := &models.ChunkFile{Deltas: []*model.DeltaModel{createOp.Delta}}

		batchFiles := &batchFiles{
			CoreIndex:        cif,
			ProvisionalIndex: pif,
			Chunk:            cf,
		}

		anchoredOps, err := provider.assembleBatchOperations(batchFiles, &txn.SidetreeTxn{Namespace: defaultNS})
		require.Error(t, err)
		require.Nil(t, anchoredOps)
		require.Contains(t, err.Error(),
			"check for duplicate suffixes in core/provisional index files: duplicate values found [deactivate-3 update-2]")
	})

	t.Run("error - invalid delta", func(t *testing.T) {
		provider := NewOperationProvider(p, operationparser.New(p), nil, nil)

		createOp, err := generateOperation(1, operation.TypeCreate)
		require.NoError(t, err)

		cif := &models.CoreIndexFile{
			ProvisionalIndexFileURI: "hash",
			Operations: models.CoreOperations{
				Create: []models.CreateOperation{{SuffixData: createOp.SuffixData}},
			},
		}

		pif := &models.ProvisionalIndexFile{
			Chunks: []models.Chunk{},
		}

		cf := &models.ChunkFile{Deltas: []*model.DeltaModel{{Patches: []patch.Patch{}}}}

		batchFiles := &batchFiles{
			CoreIndex:        cif,
			ProvisionalIndex: pif,
			Chunk:            cf,
		}

		anchoredOps, err := provider.assembleBatchOperations(batchFiles, &txn.SidetreeTxn{Namespace: defaultNS})
		require.Error(t, err)
		require.Nil(t, anchoredOps)
		require.Contains(t, err.Error(), "validate delta: missing patches")
	})
}

func newMockProtocolClient() *mocks.MockProtocolClient {
	pc := mocks.NewMockProtocolClient()
	parser := operationparser.New(pc.Protocol)
	dc := doccomposer.New()

	pv := pc.CurrentVersion
	pv.OperationParserReturns(parser)
	pv.DocumentComposerReturns(dc)

	return pc
}
