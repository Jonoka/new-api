package setting

import (
	"github.com/QuantumNous/new-api/common"
)

var (
	BepusdtApiUrl    string
	BepusdtAuthToken string
	BepusdtUnitPrice float64 = 7.2
	BepusdtMinTopUp  int     = 1
	BepusdtTimeout   int     = 1200
	BepusdtChains    string  = "[]"
)

type BepusdtChain struct {
	Name      string `json:"name"`       // Display name, e.g. "TRC20"
	TradeType string `json:"trade_type"` // bepusdt trade_type, e.g. "usdt.trc20"
}

func GetBepusdtChains() []BepusdtChain {
	if BepusdtChains == "" || BepusdtChains == "[]" {
		return nil
	}
	var chains []BepusdtChain
	if err := common.UnmarshalJsonStr(BepusdtChains, &chains); err != nil {
		return nil
	}
	return chains
}

func BepusdtChains2JsonString() string {
	return BepusdtChains
}
