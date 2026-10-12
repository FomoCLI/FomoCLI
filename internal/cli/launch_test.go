package cli

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/FomoCLI/FomoCLI/internal/pons"
)

const testKey = "4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

var (
	fakeToken = common.HexToAddress("0x1111111111111111111111111111111111111111")
	fakeCurve = common.HexToAddress("0x2222222222222222222222222222222222222222")
)

type revertErr struct{ data string }

func (e revertErr) Error() string          { return "execution reverted" }
func (e revertErr) ErrorData() interface{} { return e.data }

var _ rpc.DataError = revertErr{}

// fakeChain answers the factory reads the launcher makes.
type fakeChain struct {
	whitelisted bool
	revertWith  string // custom error name to revert launches with
	sent        []*types.Transaction
}

func (f *fakeChain) ChainID(context.Context) (*big.Int, error) { return big.NewInt(pons.ChainID), nil }
func (f *fakeChain) BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	return big.NewInt(1e18), nil
}
func (f *fakeChain) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return 3_500_000, nil
}
func (f *fakeChain) SuggestGasTipCap(context.Context) (*big.Int, error) { return big.NewInt(1), nil }
func (f *fakeChain) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return &types.Header{BaseFee: big.NewInt(2e7)}, nil
}
func (f *fakeChain) PendingNonceAt(context.Context, common.Address) (uint64, error) { return 7, nil }
func (f *fakeChain) SendTransaction(_ context.Context, tx *types.Transaction) error {
	f.sent = append(f.sent, tx)
	return nil
}
func (f *fakeChain) TransactionReceipt(_ context.Context, h common.Hash) (*types.Receipt, error) {
	ev := pons.FactoryABI.Events["TokenLaunched"].ID
	return &types.Receipt{Status: 1, TxHash: h, Logs: []*types.Log{{Address: pons.Factory,
		Topics: []common.Hash{ev, common.BytesToHash(fakeToken.Bytes()), common.BytesToHash(fakeCurve.Bytes()), {}}}}}, nil
}

func (f *fakeChain) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	m, err := pons.FactoryABI.MethodById(msg.Data[:4])
	if err != nil {
		if rm, rerr := pons.RouterABI.MethodById(msg.Data[:4]); rerr == nil {
			return rm.Outputs.Pack(fakeToken, fakeCurve, big.NewInt(42))
		}
		return nil, err
	}
	switch m.Name {
	case "launchFee":
		return m.Outputs.Pack(big.NewInt(5e14))
	case "maxCreatorTaxBps":
		return m.Outputs.Pack(uint16(1000))
	case "getLaunchConfig":
		return m.Outputs.Pack(pons.LaunchConfig{Supply: new(big.Int).Mul(big.NewInt(1e9), big.NewInt(1e18)), CurveFeeBps: big.NewInt(100),
			PhantomQuote: big.NewInt(168e16), GraduationThreshold: big.NewInt(42e17), PoolFee: big.NewInt(0), TickSpacing: big.NewInt(200), Enabled: true})
	case "previewLaunchEconomics":
		return m.Outputs.Pack([32]byte{1})
	case "canLaunch":
		return m.Outputs.Pack(f.whitelisted)
	case "launchToken":
		if f.revertWith != "" {
			return nil, revertErr{"0x" + common.Bytes2Hex(crypto.Keccak256([]byte(f.revertWith + "()"))[:4])}
		}
		return m.Outputs.Pack(fakeToken, fakeCurve)
	}
	return nil, errors.New("unexpected call " + m.Name)
}

func withFakeChain(t *testing.T, f *fakeChain, answer string) {
	t.Helper()
	prevDial, prevLine, prevSecret := dialPons, readLine, readSecret
	dialPons = func(context.Context, string) (pons.Backend, error) { return f, nil }
	readLine = func(string) (string, error) { return answer, nil }
	readSecret = func(string) (string, error) { return testKey, nil }
	t.Cleanup(func() { dialPons, readLine, readSecret = prevDial, prevLine, prevSecret })
}

func TestLaunchDryRunFromProfile(t *testing.T) {
	srv := setupEnv(t, "test-key")
	f := &fakeChain{whitelisted: true}
	withFakeChain(t, f, "")
	out, e, code := run(t, srv, "launch", "CryptoKaleo", "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, e, out)
	}
	for _, w := range []string{"$CRYPTOKALE", "K A L E O", "https://example.com/kaleo.png", "trader bio",
		"0x7b4D16237683Fe1765E727eaDF99C6f02Adf0b59", "fomo.family wallet of @CryptoKaleo", "0.0005 ETH", "4.2 ETH",
		"simulation passed", fakeToken.Hex(), "Dry run"} {
		if !strings.Contains(out, w) {
			t.Errorf("preview missing %q:\n%s", w, out)
		}
	}
	if len(f.sent) != 0 {
		t.Fatal("dry run must not send")
	}
}

func TestLaunchSendsSignedTransaction(t *testing.T) {
	srv := setupEnv(t, "test-key")
	t.Setenv("FOMO_PRIVATE_KEY", "0x"+testKey) // no terminal in tests: key comes from the env
	f := &fakeChain{whitelisted: true}
	withFakeChain(t, f, "cryptokale") // wrong ticker: must cancel
	out, e, code := run(t, srv, "launch", "CryptoKaleo", "--fee-to", "self", "--symbol", "kaleo", "--tax", "1")
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, e, out)
	}
	if !strings.Contains(out, "Cancelled") || len(f.sent) != 0 {
		t.Fatalf("wrong confirmation word must cancel:\n%s", out)
	}
	// Confirm with the right ticker this time.
	withFakeChain(t, f, "KALEO")
	out, e, code = run(t, srv, "launch", "CryptoKaleo", "--fee-to", "self", "--symbol", "kaleo", "--tax", "1")
	if code != 0 || len(f.sent) != 1 {
		t.Fatalf("exit %d, sent %d: %s\n%s", code, len(f.sent), e, out)
	}
	tx := f.sent[0]
	key, _ := pons.ParseKey(testKey)
	me := crypto.PubkeyToAddress(key.PublicKey)
	from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(pons.ChainID)), tx)
	if err != nil || from != me || tx.ChainId().Int64() != pons.ChainID || *tx.To() != pons.Factory || tx.Value().Int64() != 5e14 || tx.Nonce() != 7 {
		t.Fatalf("bad tx: from %s chain %s to %s value %s", from.Hex(), tx.ChainId(), tx.To().Hex(), tx.Value())
	}
	args, err := pons.FactoryABI.Methods["launchToken"].Inputs.Unpack(tx.Data()[4:])
	if err != nil {
		t.Fatal(err)
	}
	enc := args[0].(struct {
		Name        string `json:"name"`
		Symbol      string `json:"symbol"`
		Logo        string `json:"logo"`
		Description string `json:"description"`
		Socials     struct {
			Twitter   string `json:"twitter"`
			Telegram  string `json:"telegram"`
			Discord   string `json:"discord"`
			Website   string `json:"website"`
			Farcaster string `json:"farcaster"`
		} `json:"socials"`
		CreatorFeeRecipient common.Address `json:"creatorFeeRecipient"`
		CreatorTaxBps       uint16         `json:"creatorTaxBps"`
		BuybackEnabled      bool           `json:"buybackEnabled"`
		ExpectedEconomics   [32]uint8      `json:"expectedEconomics"`
		Salt                [32]uint8      `json:"salt"`
	})
	if enc.Symbol != "KALEO" || enc.CreatorFeeRecipient != me || enc.CreatorTaxBps != 100 || enc.ExpectedEconomics != [32]byte{1} || !enc.BuybackEnabled {
		t.Errorf("encoded params wrong: %+v", enc)
	}
	if !strings.Contains(out, "LAUNCHED") || !strings.Contains(out, fakeToken.Hex()) {
		t.Errorf("success output:\n%s", out)
	}
}

func TestLaunchExplainsReverts(t *testing.T) {
	srv := setupEnv(t, "test-key")
	f := &fakeChain{whitelisted: false, revertWith: "NotWhitelisted"}
	withFakeChain(t, f, "KALEO")
	out, e, code := run(t, srv, "launch", "CryptoKaleo", "--dry-run")
	if code == 0 || !strings.Contains(e, "NotWhitelisted") || !strings.Contains(e, "whitelist") || !strings.Contains(out, "not whitelisted") {
		t.Fatalf("revert not explained (%d):\n%s\n%s", code, out, e)
	}
	if len(f.sent) != 0 {
		t.Fatal("must never send after a failed simulation")
	}
}

func TestLaunchValidation(t *testing.T) {
	srv := setupEnv(t, "test-key")
	withFakeChain(t, &fakeChain{whitelisted: true}, "")
	cases := [][]string{
		{"launch", "--name", "X", "--symbol", "X", "--dry-run", "--from", "0x01"},                     // no logo
		{"launch", "--name", "X", "--symbol", "X", "--logo", "ftp://x", "--dry-run"},                  // bad logo
		{"launch", "CryptoKaleo", "--tax", "50", "--dry-run"},                                         // tax over cap
		{"launch", "CryptoKaleo", "--pair", "USDG", "--buy", "0.1", "--dry-run"},                      // buy on custom pair
		{"launch", "CryptoKaleo", "--fee-to", "nope", "--dry-run"},                                    // bad fee-to
		{"launch", "--name", "X", "--symbol", "WAYTOOLONGTICKER", "--logo", "https://x", "--dry-run"}, // ticker
		{"--demo", "launch", "CryptoKaleo"},                                                           // demo
	}
	for _, c := range cases {
		if _, e, code := run(t, srv, c...); code != ExitUsage && code != ExitError {
			t.Errorf("%v: expected a usage error, got %d %s", c, code, e)
		}
	}
}
