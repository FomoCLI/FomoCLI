// Package pons launches tokens on the Pons v2 launchpad (Robinhood Chain,
// chain id 4663), following docs.ponsfamily.com/v2.
//
// Only the launch system is implemented: read the live launch terms, pin them,
// build a launchToken (or launchAndBuy) call, simulate it, sign it and read
// the new token back from the receipt. Pons never takes custody of funds; the
// user's own key signs every transaction.
package pons

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// Network and contract addresses (docs.ponsfamily.com/v2#contracts).
const (
	ChainID     = 4663
	DefaultRPC  = "https://rpc.mainnet.chain.robinhood.com"
	ExplorerURL = "https://robinhoodchain.blockscout.com"
)

var (
	Factory      = common.HexToAddress("0x7eD598BcEf8bd9Edd8C97A195C6d13f40801EC7e")
	LaunchAndBuy = common.HexToAddress("0xe33E9E479dF8802cb0866d5d05258bEc4cF62948")
	FeeEscrow    = common.HexToAddress("0xd3AFEB2a57f70eF218Aa82451c51B2fb0416Ac9e")
)

// QuoteToken is an ERC-20 a launch may be paired against. It is only offered
// after the factory confirms it on-chain (approvedPairTokens + economics).
type QuoteToken struct {
	Symbol, Name string
	Address      common.Address
}

// QuoteTokens are the candidate pair assets on Robinhood Chain.
var QuoteTokens = []QuoteToken{
	{"USDG", "Global Dollar", common.HexToAddress("0x5fc5360D0400a0Fd4f2af552ADD042D716F1d168")},
	{"NVDA", "NVIDIA", common.HexToAddress("0xd0601CE157Db5bdC3162BbaC2a2C8aF5320D9EEC")},
	{"SPCX", "SpaceX Class A", common.HexToAddress("0x4a0E65A3EcceC6dBe60AE065F2e7bb85Fae35eEa")},
	{"GOOGL", "Alphabet Class A", common.HexToAddress("0x2e0847E8910a9732eB3fb1bb4b70a580ADAD4FE3")},
	{"TSLA", "Tesla", common.HexToAddress("0x322F0929c4625eD5bAd873c95208D54E1c003b2d")},
	{"GME", "GameStop", common.HexToAddress("0x1b0E319c6A659F002271B69dB8A7df2F911c153E")},
	{"AAPL", "Apple", common.HexToAddress("0xaF3D76f1834A1d425780943C99Ea8A608f8a93f9")},
	{"SPY", "S&P 500 ETF", common.HexToAddress("0x117cc2133c37B721F49dE2A7a74833232B3B4C0C")},
	{"SNDK", "SanDisk", common.HexToAddress("0xB90A19fF0Af67f7779afF50A882A9CfF42446400")},
	{"AMD", "Advanced Micro Devices", common.HexToAddress("0x86923f96303D656E4aa86D9d42D1e57ad2023fdC")},
	{"AMZN", "Amazon", common.HexToAddress("0x12f190a9F9d7D37a250758b26824B97CE941bF54")},
	{"MSFT", "Microsoft", common.HexToAddress("0xe93237C50D904957Cf27E7B1133b510C669c2e74")},
	{"META", "Meta Platforms", common.HexToAddress("0xc0D6457C16Cc70d6790Dd43521C899C87ce02f35")},
	{"CRCL", "Circle", common.HexToAddress("0xdF0992E440dD0be65BD8439b609d6D4366bf1CB5")},
	{"COIN", "Coinbase", common.HexToAddress("0x6330D8C3178a418788dF01a47479c0ce7CCF450b")},
	{"MU", "Micron", common.HexToAddress("0xfF080c8ce2E5feadaCa0Da81314Ae59D232d4afD")},
	{"PLTR", "Palantir", common.HexToAddress("0x894e1ec2d74ffe5aef8dc8a9e84686accb964f2a")},
}

// LookupQuote resolves "ETH", a symbol from QuoteTokens, or a 0x address.
// The zero address means native ETH.
func LookupQuote(s string) (common.Address, string, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "ETH") {
		return common.Address{}, "ETH", nil
	}
	for _, q := range QuoteTokens {
		if strings.EqualFold(q.Symbol, s) {
			return q.Address, q.Symbol, nil
		}
	}
	if common.IsHexAddress(s) {
		return common.HexToAddress(s), s, nil
	}
	return common.Address{}, "", fmt.Errorf("unknown pair %q (use ETH, a symbol like USDG, or a token address)", s)
}

// ---------------------------------------------------------------- ABIs

const tokenParamsTuple = `{"name":"params","type":"tuple","components":[
 {"name":"name","type":"string"},{"name":"symbol","type":"string"},{"name":"logo","type":"string"},{"name":"description","type":"string"},
 {"name":"socials","type":"tuple","components":[{"name":"twitter","type":"string"},{"name":"telegram","type":"string"},{"name":"discord","type":"string"},{"name":"website","type":"string"},{"name":"farcaster","type":"string"}]},
 {"name":"creatorFeeRecipient","type":"address"},{"name":"creatorTaxBps","type":"uint16"},{"name":"buybackEnabled","type":"bool"},
 {"name":"expectedEconomics","type":"bytes32"},{"name":"salt","type":"bytes32"}]}`

const factoryJSON = `[
{"name":"launchToken","type":"function","stateMutability":"payable","inputs":[` + tokenParamsTuple + `,
  {"name":"launchConfigId","type":"uint256"},{"name":"pairToken","type":"address"},{"name":"snipeTaxExemptions","type":"address[]"}],
  "outputs":[{"name":"token","type":"address"},{"name":"curve","type":"address"}]},
{"name":"previewLaunchEconomics","type":"function","stateMutability":"view","inputs":[{"name":"launchConfigId","type":"uint256"},{"name":"pairToken","type":"address"}],"outputs":[{"type":"bytes32"}]},
{"name":"launchFee","type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
{"name":"maxCreatorTaxBps","type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"uint16"}]},
{"name":"canLaunch","type":"function","stateMutability":"view","inputs":[{"name":"account","type":"address"}],"outputs":[{"type":"bool"}]},
{"name":"launchConfigCount","type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
{"name":"getLaunchConfig","type":"function","stateMutability":"view","inputs":[{"name":"id","type":"uint256"}],"outputs":[{"name":"","type":"tuple","components":[
  {"name":"supply","type":"uint256"},{"name":"curveFeeBps","type":"uint256"},{"name":"phantomQuote","type":"uint256"},{"name":"graduationThreshold","type":"uint256"},
  {"name":"poolFee","type":"uint24"},{"name":"tickSpacing","type":"int24"},{"name":"enabled","type":"bool"}]}]},
{"name":"approvedPairTokens","type":"function","stateMutability":"view","inputs":[{"name":"pairToken","type":"address"}],"outputs":[{"type":"bool"}]},
{"name":"pairTokenEconomics","type":"function","stateMutability":"view","inputs":[{"name":"pairToken","type":"address"}],"outputs":[{"name":"phantomQuote","type":"uint256"},{"name":"graduationThreshold","type":"uint256"},{"name":"decimals","type":"uint8"}]},
{"name":"TokenLaunched","type":"event","anonymous":false,"inputs":[{"name":"token","type":"address","indexed":true},{"name":"curve","type":"address","indexed":true},{"name":"deployer","type":"address","indexed":true},{"name":"pairToken","type":"address","indexed":false},{"name":"launchConfigId","type":"uint256","indexed":false},{"name":"graduationThreshold","type":"uint256","indexed":false}]}
]`

const routerJSON = `[
{"name":"launchAndBuy","type":"function","stateMutability":"payable","inputs":[` + tokenParamsTuple + `,
  {"name":"launchConfigId","type":"uint256"},{"name":"pairToken","type":"address"},{"name":"quoteIn","type":"uint256"},{"name":"minTokensOut","type":"uint256"},
  {"name":"recipient","type":"address"},{"name":"snipeTaxExemptions","type":"address[]"}],
  "outputs":[{"name":"token","type":"address"},{"name":"curve","type":"address"},{"name":"tokensOut","type":"uint256"}]}
]`

var (
	FactoryABI = mustABI(factoryJSON)
	RouterABI  = mustABI(routerJSON)
)

func mustABI(s string) abi.ABI {
	a, err := abi.JSON(strings.NewReader(s))
	if err != nil {
		panic(err)
	}
	return a
}

// ---------------------------------------------------------------- types

// Socials are written into the token's on-chain metadata.
type Socials struct {
	Twitter   string `abi:"twitter"`
	Telegram  string `abi:"telegram"`
	Discord   string `abi:"discord"`
	Website   string `abi:"website"`
	Farcaster string `abi:"farcaster"`
}

// TokenParams mirrors the factory's TokenParams struct.
type TokenParams struct {
	Name                string         `abi:"name"`
	Symbol              string         `abi:"symbol"`
	Logo                string         `abi:"logo"`
	Description         string         `abi:"description"`
	Socials             Socials        `abi:"socials"`
	CreatorFeeRecipient common.Address `abi:"creatorFeeRecipient"`
	CreatorTaxBps       uint16         `abi:"creatorTaxBps"`
	BuybackEnabled      bool           `abi:"buybackEnabled"`
	ExpectedEconomics   [32]byte       `abi:"expectedEconomics"`
	Salt                [32]byte       `abi:"salt"`
}

// LaunchConfig mirrors the factory's LaunchConfig struct.
type LaunchConfig struct {
	Supply              *big.Int `abi:"supply"`
	CurveFeeBps         *big.Int `abi:"curveFeeBps"`
	PhantomQuote        *big.Int `abi:"phantomQuote"`
	GraduationThreshold *big.Int `abi:"graduationThreshold"`
	PoolFee             *big.Int `abi:"poolFee"`
	TickSpacing         *big.Int `abi:"tickSpacing"`
	Enabled             bool     `abi:"enabled"`
}

// Call is a ready-to-send contract call.
type Call struct {
	To     common.Address
	Data   []byte
	Value  *big.Int
	Method string // "launchToken" | "launchAndBuy"
}

// Launch is what the user asked for.
type Launch struct {
	Params   TokenParams
	ConfigID *big.Int
	Pair     common.Address // zero = native ETH
	BuyWei   *big.Int       // optional atomic first buy (native pair only)
	Buyer    common.Address // recipient of the first buy
}

// Build encodes the launch into a factory or router call. fee is launchFee().
func (l Launch) Build(fee *big.Int) (Call, error) {
	if l.Params.CreatorFeeRecipient == (common.Address{}) {
		return Call{}, fmt.Errorf("creator fee recipient must be set (the factory rejects the zero address)")
	}
	cfg := l.ConfigID
	if cfg == nil {
		cfg = new(big.Int)
	}
	exempt := []common.Address{}
	if l.BuyWei != nil && l.BuyWei.Sign() > 0 {
		if l.Pair != (common.Address{}) {
			return Call{}, fmt.Errorf("a first buy is only supported for ETH launches")
		}
		// minTokensOut 0: the create and the buy share one transaction, so
		// nothing can trade in between and the buy cannot be front-run.
		data, err := RouterABI.Pack("launchAndBuy", l.Params, cfg, l.Pair, l.BuyWei, new(big.Int), l.Buyer, exempt)
		if err != nil {
			return Call{}, err
		}
		return Call{To: LaunchAndBuy, Data: data, Value: new(big.Int).Add(fee, l.BuyWei), Method: "launchAndBuy"}, nil
	}
	// The 4-argument launchToken (selector 0xa72101af); the 3-argument
	// overload reverts on the deployed factory.
	data, err := FactoryABI.Pack("launchToken", l.Params, cfg, l.Pair, exempt)
	if err != nil {
		return Call{}, err
	}
	return Call{To: Factory, Data: data, Value: new(big.Int).Set(fee), Method: "launchToken"}, nil
}

// Result is the outcome of a (simulated or mined) launch.
type Result struct {
	Token, Curve common.Address
	TokensOut    *big.Int // launchAndBuy only
}

// DecodeResult unpacks the return data of a launch call.
func DecodeResult(method string, out []byte) (Result, error) {
	var a abi.ABI
	switch method {
	case "launchAndBuy":
		a = RouterABI
	default:
		a = FactoryABI
	}
	v, err := a.Unpack(method, out)
	if err != nil {
		return Result{}, err
	}
	r := Result{Token: v[0].(common.Address), Curve: v[1].(common.Address)}
	if len(v) > 2 {
		r.TokensOut = v[2].(*big.Int)
	}
	return r, nil
}
