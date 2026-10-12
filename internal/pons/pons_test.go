package pons

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestSelectorsMatchTheDeployedFactory(t *testing.T) {
	// 0xa72101af is the selector of a successful on-chain launch; the
	// 3-argument overload (0xf35abbcf) reverts.
	if got := hex.EncodeToString(FactoryABI.Methods["launchToken"].ID); got != "a72101af" {
		t.Fatalf("launchToken selector = %s, want a72101af", got)
	}
}

func params() TokenParams {
	return TokenParams{Name: "Kaleo", Symbol: "KALEO", Logo: "https://x/y.png", Description: "d",
		CreatorFeeRecipient: common.HexToAddress("0x7b4d16237683fe1765e727eadf99c6f02adf0b59"), BuybackEnabled: true, Salt: NewSalt()}
}

func TestBuild(t *testing.T) {
	fee := big.NewInt(1e15)
	c, err := Launch{Params: params()}.Build(fee)
	if err != nil || c.To != Factory || c.Value.Cmp(fee) != 0 || c.Method != "launchToken" {
		t.Fatalf("launchToken call wrong: %+v %v", c, err)
	}
	args, err := FactoryABI.Methods["launchToken"].Inputs.Unpack(c.Data[4:])
	if err != nil || len(args) != 4 {
		t.Fatalf("round trip: %v", err)
	}

	buy := big.NewInt(5e16)
	c, err = Launch{Params: params(), BuyWei: buy, Buyer: common.HexToAddress("0x01")}.Build(fee)
	if err != nil || c.To != LaunchAndBuy || c.Value.Cmp(new(big.Int).Add(fee, buy)) != 0 {
		t.Fatalf("launchAndBuy call wrong: %+v %v", c, err)
	}

	p := params()
	p.CreatorFeeRecipient = common.Address{}
	if _, err := (Launch{Params: p}).Build(fee); err == nil {
		t.Error("zero fee recipient must be rejected")
	}
	if _, err := (Launch{Params: params(), Pair: common.HexToAddress("0x5fc5"), BuyWei: buy}).Build(fee); err == nil {
		t.Error("first buy on a custom pair must be rejected")
	}
}

func TestLookupQuote(t *testing.T) {
	if a, s, err := LookupQuote("eth"); err != nil || a != (common.Address{}) || s != "ETH" {
		t.Error("ETH")
	}
	if a, _, err := LookupQuote("usdg"); err != nil || a != QuoteTokens[0].Address {
		t.Error("USDG")
	}
	if _, _, err := LookupQuote("NOPE"); err == nil {
		t.Error("unknown symbol accepted")
	}
}

func TestParseKey(t *testing.T) {
	if _, err := ParseKey("0x" + "4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseKey("abc"); err == nil {
		t.Error("short key accepted")
	}
}
