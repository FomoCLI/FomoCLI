package cli

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/FomoCLI/FomoCLI/internal/api"
	"github.com/FomoCLI/FomoCLI/internal/jx"
	"github.com/FomoCLI/FomoCLI/internal/pons"
	"github.com/FomoCLI/FomoCLI/internal/ui"
)

// dialPons connects to Robinhood Chain. Tests replace it with a fake chain.
var dialPons = func(ctx context.Context, rpcURL string) (pons.Backend, error) {
	return ethclient.DialContext(ctx, rpcURL)
}

// readSecret reads the private key without echo. Tests replace it.
var readSecret = func(prompt string) (string, error) {
	fmt.Fprint(ui.Err, prompt)
	if ui.IsTTY(os.Stdin) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(ui.Err)
		return string(b), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line), err
}

// readLine reads a confirmation answer. Tests replace it.
var readLine = func(prompt string) (string, error) {
	fmt.Fprint(ui.Err, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line), err
}

type launchOpts struct {
	name, symbol, description, logo string
	twitter, telegram, website      string
	feeTo, pair, rpc, from, buy     string
	taxPct                          float64
	noBuyback, dryRun, yes          bool
	configID                        int64
}

func launchCmd() *cobra.Command {
	var o launchOpts
	cmd := &cobra.Command{
		Use:   "launch [handle]",
		Short: "Launch your fomo.family profile as a token on Pons v2",
		Long: `Turns a fomo.family profile into a token on Pons v2 (Robinhood Chain).

The profile fills in the launch: display name as the token name, handle as the
ticker, avatar as the logo, bio as the description, and the profile's own EVM
wallet as the creator-fee recipient. Every field can be overridden.

The token starts on a Pons bonding curve holding the whole supply and
graduates into a Uniswap v4 pool with permanently locked liquidity once the
curve sells out. Pons never holds your funds: you sign the launch yourself.

Before anything is signed fomocli reads the live launch terms, pins them,
simulates the exact transaction and shows you the token address it will
create. Nothing is sent until you type the ticker to confirm.

The private key is read from FOMO_PRIVATE_KEY or a hidden prompt, used once to
sign, and never stored. Use a wallet that holds only what the launch needs.

Pons v2 is unaudited (docs.ponsfamily.com/v2#audits). Not financial advice.`,
		Example: `  fomocli launch CryptoKaleo --dry-run          # preview + simulate, no key needed
  fomocli launch CryptoKaleo                    # launch, fees to the profile's wallet
  fomocli launch CryptoKaleo --fee-to self --buy 0.02
  fomocli launch --name "My Coin" --symbol MYC --logo https://... --description "..."`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			handle := ""
			if len(args) == 1 {
				handle = cleanHandle(args[0])
			}
			return runLaunch(cmd, handle, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.name, "name", "", "token name (default: profile display name)")
	f.StringVar(&o.symbol, "symbol", "", "ticker, A-Z 0-9, max 10 (default: handle)")
	f.StringVar(&o.description, "description", "", "description (default: first line of the bio)")
	f.StringVar(&o.logo, "logo", "", "logo image URL, https:// or ipfs:// (default: profile picture)")
	f.StringVar(&o.twitter, "twitter", "", "X / Twitter link or handle")
	f.StringVar(&o.telegram, "telegram", "", "Telegram link")
	f.StringVar(&o.website, "website", "", "website link")
	f.StringVar(&o.feeTo, "fee-to", "", "creator fees to: profile | self | 0x address (default: profile wallet, else self)")
	f.Float64Var(&o.taxPct, "tax", 0, "optional creator tax in percent, capped by Pons (e.g. 1 = 1%)")
	f.BoolVar(&o.noBuyback, "no-buyback", false, "disable buybacks from your creator fees")
	f.StringVar(&o.pair, "pair", "ETH", "quote asset: ETH, an approved symbol (USDG, NVDA, ...) or address")
	f.Int64Var(&o.configID, "config", 0, "Pons launch config id")
	f.StringVar(&o.buy, "buy", "", "also buy this much ETH of your token in the same transaction (front-run proof)")
	f.BoolVar(&o.dryRun, "dry-run", false, "preview and simulate only; never asks for a key or sends")
	f.StringVar(&o.from, "from", "", "--dry-run: simulate as this address")
	f.BoolVarP(&o.yes, "yes", "y", false, "skip the confirmation prompt")
	f.StringVar(&o.rpc, "rpc", "", "Robinhood Chain RPC URL (default "+pons.DefaultRPC+", or FOMO_RPC_URL)")
	return cmd
}

var symbolClean = regexp.MustCompile(`[^A-Z0-9]`)

// launchDraft is the profile-derived launch before overrides are validated.
type launchDraft struct {
	handle, name, symbol, description, logo string
	twitter, telegram, website              string
	profileWallet                           common.Address
}

func draftFromProfile(handle string, v jx.V) launchDraft {
	d := launchDraft{handle: v.First("handle", "userHandle").Str()}
	if d.handle == "" {
		d.handle = handle
	}
	d.name = strings.TrimSpace(v.Get("displayName").Str())
	if d.name == "" {
		d.name = d.handle
	}
	d.symbol = symbolClean.ReplaceAllString(strings.ToUpper(d.handle), "")
	if len(d.symbol) > 10 {
		d.symbol = d.symbol[:10]
	}
	bio := strings.TrimSpace(strings.Split(v.Get("description").Str(), "\n")[0])
	if bio == "" {
		bio = "@" + d.handle + " on fomo.family"
	}
	d.description = truncRunes(bio, 280)
	for _, k := range []string{"profilePictureLink", "profilePicture", "avatar", "image"} {
		if s := v.Get(k).Str(); strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
			d.logo = s
			break
		}
	}
	if x := v.First("twitter", "x").Str(); x != "" {
		d.twitter = x
	}
	if w := v.Get("wallets", "evm").Str(); common.IsHexAddress(w) {
		d.profileWallet = common.HexToAddress(w)
	}
	return d
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func runLaunch(cmd *cobra.Command, handle string, o launchOpts) error {
	ctx := cmd.Context()
	if g.demo {
		return usagef("launch talks to the real chain; it is not available with --demo")
	}

	// ---- 1. profile → draft
	d := launchDraft{handle: handle}
	if handle != "" {
		res, err := fetch(cmd, "resolving @"+handle, func(ctx context.Context) (*api.Response, error) {
			if looksLikeID(handle) {
				return client.UserByID(ctx, handle)
			}
			return client.User(ctx, handle)
		})
		if err != nil {
			return err
		}
		d = draftFromProfile(handle, res.Data)
		if !g.json {
			footer(res)
		}
	}
	override := func(dst *string, v string) {
		if strings.TrimSpace(v) != "" {
			*dst = strings.TrimSpace(v)
		}
	}
	override(&d.name, o.name)
	override(&d.description, o.description)
	override(&d.logo, o.logo)
	override(&d.twitter, o.twitter)
	override(&d.telegram, o.telegram)
	override(&d.website, o.website)
	if o.symbol != "" {
		d.symbol = symbolClean.ReplaceAllString(strings.ToUpper(o.symbol), "")
	}
	if err := validateDraft(d); err != nil {
		return err
	}

	// ---- 2. chain + signer
	rpcURL := o.rpc
	if rpcURL == "" {
		rpcURL = os.Getenv("FOMO_RPC_URL")
	}
	if rpcURL == "" {
		rpcURL = pons.DefaultRPC
	}
	backend, err := dialPons(ctx, rpcURL)
	if err != nil {
		return fmt.Errorf("cannot reach Robinhood Chain: %w", err)
	}
	pc := &pons.Client{B: backend}
	if err := pc.CheckChain(ctx); err != nil {
		return err
	}

	var key *ecdsa.PrivateKey
	var signer common.Address
	switch {
	case o.dryRun && o.from != "":
		if !common.IsHexAddress(o.from) {
			return usagef("--from must be an address")
		}
		signer = common.HexToAddress(o.from)
	case o.dryRun:
		signer = d.profileWallet // best effort: simulate as the profile wallet
	default:
		key, err = loadKey()
		if err != nil {
			return err
		}
		signer = crypto.PubkeyToAddress(key.PublicKey)
	}

	// ---- 3. fee recipient, pair, tax, buy
	feeTo, feeLabel, err := resolveFeeTo(o.feeTo, d, signer)
	if err != nil {
		return err
	}
	if feeTo == (common.Address{}) {
		return usagef("no wallet to simulate from: pass --from 0x... (or --fee-to 0x...) with --dry-run")
	}
	if signer == (common.Address{}) {
		signer = feeTo
	}
	pair, pairLabel, err := pons.LookupQuote(o.pair)
	if err != nil {
		return usagef("%s", err)
	}
	var buyWei *big.Int
	if o.buy != "" {
		if buyWei, err = parseEth(o.buy); err != nil {
			return usagef("--buy: %s", err)
		}
	}
	configID := big.NewInt(o.configID)

	// ---- 4. live terms, pinned
	sp := ui.Spin("reading Pons v2 launch terms")
	terms, err := pc.ReadTerms(ctx, configID, pair, signer)
	sp.Stop()
	if err != nil {
		return err
	}
	taxBps := uint16(o.taxPct*100 + 0.5)
	if o.taxPct < 0 || (terms.MaxTaxBps > 0 && taxBps > terms.MaxTaxBps) {
		return usagef("--tax must be between 0 and %.2f%%", float64(terms.MaxTaxBps)/100)
	}

	params := pons.TokenParams{
		Name: d.name, Symbol: d.symbol, Logo: d.logo, Description: d.description,
		Socials:             pons.Socials{Twitter: d.twitter, Telegram: d.telegram, Website: d.website},
		CreatorFeeRecipient: feeTo,
		CreatorTaxBps:       taxBps,
		BuybackEnabled:      !o.noBuyback,
		ExpectedEconomics:   terms.Economics,
		Salt:                pons.NewSalt(),
	}
	launch := pons.Launch{Params: params, ConfigID: configID, Pair: pair, BuyWei: buyWei, Buyer: signer}
	call, err := launch.Build(terms.Fee)
	if err != nil {
		return usagef("%s", err)
	}

	// ---- 5. simulate the exact transaction
	sp = ui.Spin("simulating the launch")
	sim, simErr := pc.Simulate(ctx, call, signer)
	var gas uint64
	var gasCost *big.Int
	if simErr == nil {
		if gas, err = pc.EstimateGas(ctx, call, signer); err == nil {
			if _, feeCap, ferr := pc.Fees(ctx); ferr == nil {
				gasCost = new(big.Int).Mul(feeCap, new(big.Int).SetUint64(gas))
			}
		}
	}
	sp.Stop()

	warnings := launchWarnings(terms, signer, feeTo, call, gasCost)
	if g.json && o.dryRun {
		return printLaunchJSON(d, params, terms, call, sim, simErr, signer, pairLabel, warnings, "")
	}
	if !g.json {
		renderLaunchPreview(d, params, terms, call, sim, simErr, signer, feeLabel, pairLabel, gasCost, warnings)
	}
	if simErr != nil {
		if h := pons.ErrorHint(simErr); h != "" {
			return fmt.Errorf("%w - %s", simErr, h)
		}
		return simErr
	}
	if o.dryRun {
		if !g.json {
			p("\n  " + ui.Muted.Render("Dry run: nothing was signed or sent. Drop --dry-run to launch."))
		}
		return nil
	}

	// ---- 6. confirm, sign, send
	if !o.yes {
		ans, err := readLine("\n  Type " + ui.Bold.Render(d.symbol) + " to launch, anything else to cancel: ")
		if err != nil || !strings.EqualFold(strings.TrimSpace(ans), d.symbol) {
			p("  " + ui.Muted.Render("Cancelled. Nothing was sent."))
			return nil
		}
	}
	sp = ui.Spin("signing and sending")
	tx, err := pc.Send(ctx, call, key, gas)
	sp.Stop()
	if err != nil {
		if h := pons.ErrorHint(err); h != "" {
			return fmt.Errorf("%w - %s", err, h)
		}
		return err
	}
	if !g.json {
		p("\n  " + ui.Muted.Render("sent ") + ui.Bold.Render(tx.Hash().Hex()))
		p("  " + ui.Muted.Render(pons.ExplorerURL+"/tx/"+tx.Hash().Hex()))
	}
	wctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	sp = ui.Spin("waiting for Robinhood Chain to confirm")
	rcpt, err := pc.Wait(wctx, tx.Hash())
	sp.Stop()
	if err != nil {
		return fmt.Errorf("sent, but no receipt yet (%v) - check the explorer link above", err)
	}
	if rcpt.Status != 1 {
		return fmt.Errorf("the launch transaction reverted on-chain (gas used %d) - see the explorer link above", rcpt.GasUsed)
	}
	got, ok := pons.LaunchedFromReceipt(rcpt)
	if !ok {
		got = sim
	}
	if g.json {
		return printLaunchJSON(d, params, terms, call, got, nil, signer, pairLabel, warnings, tx.Hash().Hex())
	}
	p()
	p(indent(ui.Box.Render(
		ui.Badge.Render("LAUNCHED")+"  "+ui.Bold.Render("$"+d.symbol)+"  "+ui.Muted.Render(d.name)+"\n\n"+
			ui.KV(
				[2]string{"token", ui.Bold.Render(got.Token.Hex())},
				[2]string{"curve", got.Curve.Hex()},
				[2]string{"tx", tx.Hash().Hex()},
				[2]string{"explorer", pons.ExplorerURL + "/token/" + got.Token.Hex()},
			)), 2))
	p("\n  " + ui.Muted.Render("Next: ") + ui.Bold.Render("fomocli token "+got.Token.Hex()) +
		ui.Muted.Render("  ·  share it: ") + ui.Bold.Render("fomocli card "+orDefault(d.handle, "<handle>")))
	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func validateDraft(d launchDraft) error {
	switch {
	case d.name == "":
		return usagef("a token name is required: pass a handle or --name")
	case len([]rune(d.name)) > 32:
		return usagef("token name is longer than 32 characters; shorten it with --name")
	case d.symbol == "":
		return usagef("a ticker is required: pass a handle or --symbol (A-Z, 0-9)")
	case len(d.symbol) > 10:
		return usagef("ticker %q is longer than 10 characters; set --symbol", d.symbol)
	case d.logo == "":
		return usagef("no logo: the profile has no picture, pass --logo https://...")
	case !strings.HasPrefix(d.logo, "https://") && !strings.HasPrefix(d.logo, "http://") && !strings.HasPrefix(d.logo, "ipfs://"):
		return usagef("--logo must be an https:// or ipfs:// URL")
	}
	return nil
}

func resolveFeeTo(flag string, d launchDraft, signer common.Address) (common.Address, string, error) {
	switch strings.ToLower(strings.TrimSpace(flag)) {
	case "":
		if d.profileWallet != (common.Address{}) {
			return d.profileWallet, "fomo.family wallet of @" + d.handle, nil
		}
		return signer, "your signing wallet", nil
	case "profile":
		if d.profileWallet == (common.Address{}) {
			return common.Address{}, "", usagef("--fee-to profile: this profile has no EVM wallet")
		}
		return d.profileWallet, "fomo.family wallet of @" + d.handle, nil
	case "self", "me":
		if signer == (common.Address{}) {
			return common.Address{}, "", usagef("--fee-to self needs a signer; with --dry-run also pass --from")
		}
		return signer, "your signing wallet", nil
	}
	if !common.IsHexAddress(flag) {
		return common.Address{}, "", usagef("--fee-to must be profile, self or a 0x address")
	}
	return common.HexToAddress(flag), "custom address", nil
}

func loadKey() (*ecdsa.PrivateKey, error) {
	raw := os.Getenv("FOMO_PRIVATE_KEY")
	if raw == "" {
		if !ui.IsTTY(os.Stdin) {
			return nil, usagef("set FOMO_PRIVATE_KEY, or run in a terminal to be prompted (or use --dry-run)")
		}
		p("  " + ui.Gold.Render("The key signs this one transaction and is never saved. Use a wallet holding only what the launch needs."))
		var err error
		raw, err = readSecret("  Private key of the launching wallet (hidden): ")
		if err != nil {
			return nil, err
		}
	}
	return pons.ParseKey(raw)
}

// parseEth parses a decimal ETH amount into wei.
func parseEth(s string) (*big.Int, error) {
	f, ok := new(big.Float).SetPrec(256).SetString(strings.TrimSpace(s))
	if !ok || f.Sign() < 0 {
		return nil, errors.New("not a valid ETH amount")
	}
	wei, _ := new(big.Float).Mul(f, big.NewFloat(1e18)).Int(nil)
	if wei.Sign() == 0 {
		return nil, errors.New("amount must be more than zero")
	}
	return wei, nil
}

// eth formats wei as ETH with up to 6 decimals.
func eth(wei *big.Int) string {
	if wei == nil {
		return ui.Dash
	}
	f := new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(1e18))
	s := strings.TrimRight(strings.TrimRight(f.Text('f', 6), "0"), ".")
	if s == "" || s == "-0" {
		s = "0"
	}
	return s + " ETH"
}

func launchWarnings(t *pons.Terms, signer, feeTo common.Address, call pons.Call, gasCost *big.Int) []string {
	var w []string
	if t.CanLaunch != nil && !*t.CanLaunch {
		w = append(w, "This wallet is not whitelisted: Pons will reject the launch (only gas is spent). Ask Pons to whitelist it.")
	}
	if feeTo != signer && signer != (common.Address{}) {
		w = append(w, "Creator fees go to "+feeTo.Hex()+", not the wallet that signs. Only that wallet can claim them.")
	}
	if t.Balance != nil {
		need := new(big.Int).Set(call.Value)
		if gasCost != nil {
			need.Add(need, gasCost)
		}
		if t.Balance.Cmp(need) < 0 {
			w = append(w, "Balance "+eth(t.Balance)+" is below the "+eth(need)+" this launch needs (fee + buy + gas).")
		}
	}
	return w
}

func renderLaunchPreview(d launchDraft, pr pons.TokenParams, t *pons.Terms, call pons.Call, sim pons.Result, simErr error,
	signer common.Address, feeLabel, pairLabel string, gasCost *big.Int, warnings []string) {
	var socials []string
	for _, s := range [][2]string{{"x", pr.Socials.Twitter}, {"tg", pr.Socials.Telegram}, {"web", pr.Socials.Website}} {
		if s[1] != "" {
			socials = append(socials, s[0]+" "+s[1])
		}
	}
	buyback := "on"
	if !pr.BuybackEnabled {
		buyback = "off"
	}
	grad := eth(t.Config.GraduationThreshold)
	if call.Method == "launchToken" && t.PairThresh != nil {
		grad = new(big.Float).Quo(new(big.Float).SetInt(t.PairThresh), new(big.Float).SetFloat64(pow10(int(t.PairDec)))).Text('f', 2) + " " + pairLabel
	}
	supply := new(big.Int).Div(t.Config.Supply, big.NewInt(1e18))
	first := ""
	if call.Method == "launchAndBuy" {
		buy := new(big.Int).Sub(call.Value, t.Fee)
		first = eth(buy) + " in the same transaction"
	}
	head := ui.Bold.Render("PONS V2 LAUNCH") + ui.Muted.Render("  ·  Robinhood Chain")
	body := ui.KV(
		[2]string{"name", ui.Bold.Render(pr.Name)},
		[2]string{"ticker", ui.Bold.Render("$" + pr.Symbol)},
		[2]string{"logo", ui.Truncate(pr.Logo, 64)},
		[2]string{"bio", ui.Truncate(pr.Description, 64)},
		[2]string{"socials", strings.Join(socials, "  ")},
		[2]string{"fees to", pr.CreatorFeeRecipient.Hex() + ui.Muted.Render("  "+feeLabel)},
		[2]string{"tax", fmt.Sprintf("%.2f%%", float64(pr.CreatorTaxBps)/100) + ui.Muted.Render("   buyback "+buyback)},
		[2]string{"pair", pairLabel},
	)
	econ := ui.KV(
		[2]string{"supply", ui.Commas(float64(supply.Int64()), 0) + ui.Muted.Render(" tokens, all on the curve")},
		[2]string{"curve fee", fmt.Sprintf("%.2f%%", float64(t.Config.CurveFeeBps.Int64())/100)},
		[2]string{"graduates", "at " + grad + ui.Muted.Render(" → Uniswap v4, liquidity locked forever")},
		[2]string{"launch fee", eth(t.Fee)},
		[2]string{"first buy", first},
		[2]string{"gas", gasText(gasCost)},
		[2]string{"total", eth(sumWei(call.Value, gasCost))},
	)
	who := ui.KV(
		[2]string{"signer", signerText(signer)},
		[2]string{"balance", balText(t.Balance)},
	)
	p()
	p(indent(ui.Box.Render(head+"\n\n"+body+"\n\n"+econ+"\n\n"+who), 2))
	if simErr == nil {
		p("\n  " + ui.Up.Render("✔ simulation passed") + ui.Muted.Render("  token will be ") + ui.Bold.Render(sim.Token.Hex()))
	} else {
		p("\n  " + ui.Down.Render("✖ simulation failed: ") + simErr.Error())
	}
	for _, w := range warnings {
		p("  " + ui.Gold.Render("! ") + w)
	}
	p("  " + ui.Muted.Render("Pons v2 is unaudited. Launch tokens are volatile and can lose all value."))
}

func pow10(n int) float64 {
	f := 1.0
	for i := 0; i < n; i++ {
		f *= 10
	}
	return f
}

func sumWei(a, b *big.Int) *big.Int {
	s := new(big.Int).Set(a)
	if b != nil {
		s.Add(s, b)
	}
	return s
}

func gasText(c *big.Int) string {
	if c == nil {
		return ui.Muted.Render("unknown until simulation passes")
	}
	return "≈ " + eth(c) + ui.Muted.Render(" max")
}

func signerText(a common.Address) string {
	if a == (common.Address{}) {
		return ui.Muted.Render("none (dry run)")
	}
	return a.Hex()
}

func balText(b *big.Int) string {
	if b == nil {
		return ui.Dash
	}
	return eth(b)
}

func printLaunchJSON(d launchDraft, pr pons.TokenParams, t *pons.Terms, call pons.Call, r pons.Result, simErr error,
	signer common.Address, pairLabel string, warnings []string, txHash string) error {
	out := map[string]any{
		"name": pr.Name, "symbol": pr.Symbol, "logo": pr.Logo, "description": pr.Description,
		"creatorFeeRecipient": pr.CreatorFeeRecipient.Hex(), "creatorTaxBps": pr.CreatorTaxBps,
		"buybackEnabled": pr.BuybackEnabled, "pair": pairLabel, "method": call.Method,
		"to": call.To.Hex(), "valueWei": call.Value.String(), "launchFeeWei": t.Fee.String(),
		"signer": signer.Hex(), "warnings": warnings, "chainId": pons.ChainID,
	}
	if simErr != nil {
		out["simulationError"] = simErr.Error()
	} else {
		out["token"], out["curve"] = r.Token.Hex(), r.Curve.Hex()
	}
	if txHash != "" {
		out["tx"] = txHash
		out["explorer"] = pons.ExplorerURL + "/token/" + r.Token.Hex()
	}
	if d.handle != "" {
		out["handle"] = d.handle
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	p(string(b))
	if simErr != nil {
		return simErr
	}
	return nil
}
