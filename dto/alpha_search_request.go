package dto

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// AlphaSearchRequest preserves the complete synchronous Alpha JSON request.
type AlphaSearchRequest struct {
	Model   string          `json:"model"`
	Stream  *bool           `json:"stream,omitempty"`
	RawBody json.RawMessage `json:"-"`
}

func (r *AlphaSearchRequest) GetTokenCountMeta() *types.TokenCountMeta {
	return &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer}
}

func (r *AlphaSearchRequest) IsStream(*gin.Context) bool {
	return false
}

func (r *AlphaSearchRequest) SetModelName(modelName string) {
	r.Model = modelName
}
