/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operation

import (
	"encoding/json"

	"github.com/trustbloc/sidetree-core-go/pkg/api/batch"
	"github.com/trustbloc/sidetree-core-go/pkg/api/protocol"
	"github.com/trustbloc/sidetree-core-go/pkg/docutil"
	"github.com/trustbloc/sidetree-core-go/pkg/restapi/model"
)

// ParseUpdateOperation will parse update operation
func ParseUpdateOperation(request []byte, protocol protocol.Protocol) (*batch.Operation, error) {
	schema, err := parseUpdateRequest(request)
	if err != nil {
		return nil, err
	}

	patchData, err := parseUpdatePatchData(schema.PatchData, protocol.HashAlgorithmInMultiHashCode)
	if err != nil {
		return nil, err
	}

	return &batch.Operation{
		Type:                         batch.OperationTypeUpdate,
		OperationBuffer:              request,
		UniqueSuffix:                 schema.DidUniqueSuffix,
		PatchData:                    patchData,
		EncodedPatchData:             schema.PatchData,
		UpdateRevealValue:            schema.UpdateRevealValue,
		NextUpdateCommitmentHash:     patchData.NextUpdateCommitmentHash,
		HashAlgorithmInMultiHashCode: protocol.HashAlgorithmInMultiHashCode,
		SignedData:                   schema.SignedData,
	}, nil
}

func parseUpdateRequest(payload []byte) (*model.UpdateRequest, error) {
	schema := &model.UpdateRequest{}
	err := json.Unmarshal(payload, schema)
	if err != nil {
		return nil, err
	}
	return schema, nil
}

func parseUpdatePatchData(encoded string, code uint) (*model.PatchDataModel, error) {
	bytes, err := docutil.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	schema := &model.PatchDataModel{}
	err = json.Unmarshal(bytes, schema)
	if err != nil {
		return nil, err
	}

	if err := validatePatchData(schema, code); err != nil {
		return nil, err
	}

	return schema, nil
}
