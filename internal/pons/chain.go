package pons

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
)

// Backend is the subset of an Ethereum RPC client the launcher needs.
// *ethclient.Client satisfies it; tests use a fake.
type Backend interface {
	ChainID(ctx context.Context) (*big.Int, error)
	BalanceAt(ctx context.Context, account common.Address, block *big.Int) (*big.Int, error)
	CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error)
}

// Client reads launch terms and sends launches.
type Client struct{ B Backend }

// Terms are the live launch settings read from the factory.
type Terms struct {
	Fee        *big.Int
	MaxTaxBps  uint16
	Config     LaunchConfig
	Economics  [32]byte
	CanLaunch  *bool // nil when the read failed
	Balance    *big.Int
	PairOK     bool
	PairThresh *big.Int // graduation threshold in the pair's units (custom pair)
	PairDec    uint8
}

func (c *Client) read(ctx context.Context, method string, args ...any) ([]any, error) {
	data, err := FactoryABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	out, err := c.B.CallContract(ctx, ethereum.CallMsg{To: &Factory, Data: data}, nil)
	if err != nil {
		return nil, Explain(err)
	}
	return FactoryABI.Unpack(method, out)
}

// CheckChain confirms the RPC is Robinhood Chain.
func (c *Client) CheckChain(ctx context.Context) error {
	id, err := c.B.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("cannot reach Robinhood Chain RPC: %w", err)
	}
	if id.Int64() != ChainID {
		return fmt.Errorf("RPC is chain %s, expected Robinhood Chain (%d)", id, ChainID)
	}
	return nil
}

// ReadTerms loads everything a launch preview needs.
func (c *Client) ReadTerms(ctx context.Context, configID *big.Int, pair, account common.Address) (*Terms, error) {
	t := &Terms{}
	v, err := c.read(ctx, "launchFee")
	if err != nil {
		return nil, fmt.Errorf("launchFee: %w", err)
	}
	t.Fee = v[0].(*big.Int)
	if v, err = c.read(ctx, "maxCreatorTaxBps"); err == nil {
		t.MaxTaxBps = v[0].(uint16)
	}
	v, err = c.read(ctx, "getLaunchConfig", configID)
	if err != nil {
		return nil, fmt.Errorf("launch config %s: %w", configID, err)
	}
	t.Config = *abi.ConvertType(v[0], new(LaunchConfig)).(*LaunchConfig)
	if !t.Config.Enabled {
		return t, fmt.Errorf("launch config %s is disabled; pick another with --config", configID)
	}
	v, err = c.read(ctx, "previewLaunchEconomics", configID, pair)
	if err != nil {
		return nil, fmt.Errorf("previewLaunchEconomics: %w", err)
	}
	t.Economics = v[0].([32]byte)
	if account != (common.Address{}) {
		if v, err := c.read(ctx, "canLaunch", account); err == nil {
			b := v[0].(bool)
			t.CanLaunch = &b
		}
		if bal, err := c.B.BalanceAt(ctx, account, nil); err == nil {
			t.Balance = bal
		}
	}
	t.PairOK = true
	if pair != (common.Address{}) {
		v, err := c.read(ctx, "approvedPairTokens", pair)
		t.PairOK = err == nil && v[0].(bool)
		if e, err := c.read(ctx, "pairTokenEconomics", pair); err == nil {
			t.PairThresh, t.PairDec = e[1].(*big.Int), e[2].(uint8)
			if e[0].(*big.Int).Sign() == 0 || t.PairThresh.Sign() == 0 {
				t.PairOK = false
			}
		}
		if !t.PairOK {
			return t, fmt.Errorf("pair token %s is not approved for launches", pair.Hex())
		}
	}
	return t, nil
}

// Simulate runs the call with eth_call from the given account and returns the
// addresses the launch would create (CREATE2, so they match the real launch).
func (c *Client) Simulate(ctx context.Context, call Call, from common.Address) (Result, error) {
	out, err := c.B.CallContract(ctx, ethereum.CallMsg{From: from, To: &call.To, Data: call.Data, Value: call.Value}, nil)
	if err != nil {
		return Result{}, Explain(err)
	}
	return DecodeResult(call.Method, out)
}

// EstimateGas returns a padded gas limit (+30%, as launches run ~1-4M gas).
func (c *Client) EstimateGas(ctx context.Context, call Call, from common.Address) (uint64, error) {
	g, err := c.B.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &call.To, Data: call.Data, Value: call.Value})
	if err != nil {
		return 0, Explain(err)
	}
	return g * 13 / 10, nil
}

// Fees returns an EIP-1559 tip and fee cap.
func (c *Client) Fees(ctx context.Context) (tip, feeCap *big.Int, err error) {
	tip, err = c.B.SuggestGasTipCap(ctx)
	if err != nil {
		tip = new(big.Int)
	}
	head, err := c.B.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	base := head.BaseFee
	if base == nil {
		base = big.NewInt(1e8)
	}
	feeCap = new(big.Int).Add(new(big.Int).Mul(base, big.NewInt(2)), tip)
	return tip, feeCap, nil
}

// Send signs and broadcasts the call.
func (c *Client) Send(ctx context.Context, call Call, key *ecdsa.PrivateKey, gas uint64) (*types.Transaction, error) {
	from := crypto.PubkeyToAddress(key.PublicKey)
	nonce, err := c.B.PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}
	tip, feeCap, err := c.Fees(ctx)
	if err != nil {
		return nil, err
	}
	to := call.To
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(ChainID), Nonce: nonce, GasTipCap: tip, GasFeeCap: feeCap,
		Gas: gas, To: &to, Value: call.Value, Data: call.Data,
	})
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(big.NewInt(ChainID)), key)
	if err != nil {
		return nil, err
	}
	if err := c.B.SendTransaction(ctx, signed); err != nil {
		return nil, Explain(err)
	}
	return signed, nil
}

// Wait polls for the receipt.
func (c *Client) Wait(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	t := time.NewTicker(1500 * time.Millisecond)
	defer t.Stop()
	for {
		r, err := c.B.TransactionReceipt(ctx, hash)
		if err == nil && r != nil {
			return r, nil
		}
		if err != nil && !errors.Is(err, ethereum.NotFound) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}

// LaunchedFromReceipt reads the factory's TokenLaunched event.
func LaunchedFromReceipt(r *types.Receipt) (Result, bool) {
	ev := FactoryABI.Events["TokenLaunched"]
	for _, l := range r.Logs {
		if l.Address == Factory && len(l.Topics) >= 4 && l.Topics[0] == ev.ID {
			return Result{Token: common.BytesToAddress(l.Topics[1].Bytes()), Curve: common.BytesToAddress(l.Topics[2].Bytes())}, true
		}
	}
	return Result{}, false
}

// NewSalt returns a fresh random CREATE2 salt.
func NewSalt() [32]byte {
	var s [32]byte
	if _, err := rand.Read(s[:]); err != nil {
		panic(err)
	}
	return s
}

// ParseKey reads a hex private key (with or without 0x).
func ParseKey(s string) (*ecdsa.PrivateKey, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	if len(s) != 64 {
		return nil, errors.New("private key must be 64 hex characters")
	}
	k, err := crypto.HexToECDSA(s)
	if err != nil {
		return nil, errors.New("invalid private key")
	}
	return k, nil
}

// ---------------------------------------------------------------- errors

// knownErrors are the custom errors the docs say an integration will meet.
var knownErrors = map[string]string{
	"NotWhitelisted":            "Launching is restricted to whitelisted wallets right now. Ask Pons to whitelist this address.",
	"LaunchEconomicsMismatch":   "The launch terms changed since they were read. Run the command again.",
	"LaunchFeeNotPaid":          "The launch fee sent did not match launchFee(). Run the command again.",
	"CreatorTaxTooHigh":         "The creator tax is above the protocol cap.",
	"PairTokenNotApproved":      "That pair token is not approved for launches.",
	"PairTokenDecimalsMismatch": "The pair token reports different decimals than recorded.",
	"NativeValueMismatch":       "The ETH sent did not match the buy amount.",
	"UnexpectedNativeValue":     "ETH was sent to a custom-pair launch.",
	"LaunchConfigDisabled":      "That launch config is disabled. Pick another with --config.",
	"ExemptionListTooLong":      "Too many snipe-tax exemptions (max 32).",
	"SlippageExceeded":          "The first buy could not meet its minimum output.",
	"CurveGraduated":            "The curve has already graduated.",
}

var selectorToError = func() map[string]string {
	m := map[string]string{}
	for name := range knownErrors {
		m[hex.EncodeToString(crypto.Keccak256([]byte(name + "()"))[:4])] = name
	}
	return m
}()

// LaunchError is a decoded on-chain revert.
type LaunchError struct {
	Name, Hint, Raw string
}

func (e *LaunchError) Error() string {
	if e.Name != "" {
		return "launch would revert: " + e.Name
	}
	return "launch would revert: " + e.Raw
}

// Explain turns an RPC revert into a LaunchError when it can be decoded.
func Explain(err error) error {
	if err == nil {
		return nil
	}
	var de rpc.DataError
	if errors.As(err, &de) {
		if s, ok := de.ErrorData().(string); ok {
			s = strings.TrimPrefix(s, "0x")
			if len(s) >= 8 {
				if name, ok := selectorToError[s[:8]]; ok {
					return &LaunchError{Name: name, Hint: knownErrors[name], Raw: "0x" + s}
				}
				return &LaunchError{Raw: err.Error() + " (data 0x" + s[:min(len(s), 74)] + ")"}
			}
		}
	}
	if strings.Contains(err.Error(), "execution reverted") || strings.Contains(err.Error(), "insufficient funds") {
		return &LaunchError{Raw: err.Error()}
	}
	return err
}

// ErrorHint returns the human hint for a decoded revert, if any.
func ErrorHint(err error) string {
	var le *LaunchError
	if errors.As(err, &le) {
		if le.Hint != "" {
			return le.Hint
		}
		if strings.Contains(le.Raw, "insufficient funds") {
			return "Not enough ETH on Robinhood Chain for the launch fee, the first buy and gas."
		}
	}
	return ""
}
