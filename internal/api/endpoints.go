package api

import (
	"context"
	"net/url"
	"strconv"
)

// Credit costs as documented at https://fomoapi.io/docs#credits.
const (
	CostRead       = 250
	CostAlerts     = 125
	CostThesisPage = 1250
	CostResolve    = 2500
)

// Params is a convenience builder for query strings.
type Params map[string]string

func (p Params) values() url.Values {
	q := url.Values{}
	for k, v := range p {
		if v != "" && v != "0" {
			q.Set(k, v)
		}
	}
	return q
}

// Itoa formats an int for Params, returning "" for zero so it is omitted.
func Itoa(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func esc(s string) string { return url.PathEscape(s) }

// ---- service (keyless) ----

func (c *Client) Info(ctx context.Context) (*Response, error)   { return c.Get(ctx, "/v1", nil) }
func (c *Client) Health(ctx context.Context) (*Response, error) { return c.Get(ctx, "/health", nil) }
func (c *Client) Status(ctx context.Context) (*Response, error) { return c.Get(ctx, "/status", nil) }
func (c *Client) Me(ctx context.Context) (*Response, error)     { return c.Get(ctx, "/v2/me", nil) }

// ---- leaderboards ----

func (c *Client) Leaderboard(ctx context.Context, window string, limit int) (*Response, error) {
	return c.Get(ctx, "/v2/leaderboard/"+esc(window), Params{"limit": Itoa(limit)}.values())
}

func (c *Client) TokenBoard(ctx context.Context, board string, limit int) (*Response, error) {
	return c.Get(ctx, "/v2/leaderboard/tokens/"+esc(board), Params{"limit": Itoa(limit)}.values())
}

// ---- traders ----

func (c *Client) User(ctx context.Context, handle string) (*Response, error) {
	return c.Get(ctx, "/v2/users/"+esc(handle), nil)
}

func (c *Client) UserByID(ctx context.Context, id string) (*Response, error) {
	return c.Get(ctx, "/v2/users/id/"+esc(id), nil)
}

func (c *Client) Positions(ctx context.Context, handle string, p Params) (*Response, error) {
	return c.Get(ctx, "/v2/users/"+esc(handle)+"/positions", p.values())
}

func (c *Client) Swaps(ctx context.Context, handle string, p Params) (*Response, error) {
	return c.Get(ctx, "/v2/users/"+esc(handle)+"/swaps", p.values())
}

func (c *Client) Balances(ctx context.Context, handle, chain string) (*Response, error) {
	return c.Get(ctx, "/v2/users/"+esc(handle)+"/balances", Params{"chain": chain}.values())
}

func (c *Client) Following(ctx context.Context, handle string, limit int) (*Response, error) {
	return c.Get(ctx, "/v2/users/"+esc(handle)+"/following", Params{"limit": Itoa(limit)}.values())
}

func (c *Client) Followers(ctx context.Context, handle string, limit int) (*Response, error) {
	return c.Get(ctx, "/v2/users/"+esc(handle)+"/followers", Params{"limit": Itoa(limit)}.values())
}

func (c *Client) Spotlight(ctx context.Context, handle string) (*Response, error) {
	return c.Get(ctx, "/v2/users/"+esc(handle)+"/spotlight", nil)
}

// ---- trades ----

func (c *Client) Trade(ctx context.Context, id string) (*Response, error) {
	return c.Get(ctx, "/v2/trades/"+esc(id), nil)
}

func (c *Client) TradeComments(ctx context.Context, id string, limit int) (*Response, error) {
	return c.Get(ctx, "/v2/trades/"+esc(id)+"/comments", Params{"limit": Itoa(limit)}.values())
}

// ---- tokens ----

func (c *Client) TokenStats(ctx context.Context, addr, networkID string) (*Response, error) {
	return c.Get(ctx, "/v2/token/"+esc(addr)+"/stats", Params{"networkId": networkID}.values())
}

func (c *Client) TokenDevs(ctx context.Context, addr, networkID string) (*Response, error) {
	return c.Get(ctx, "/v2/token/"+esc(addr)+"/devs", Params{"networkId": networkID}.values())
}

func (c *Client) TokenHolders(ctx context.Context, addr string, limit int) (*Response, error) {
	return c.Get(ctx, "/token/"+esc(addr)+"/holders", Params{"limit": Itoa(limit)}.values())
}

func (c *Client) TokenActivity(ctx context.Context, p Params) (*Response, error) {
	return c.Get(ctx, "/v2/tokens/activity", p.values())
}

// ---- theses ----

func (c *Client) Theses(ctx context.Context, p Params) (*Response, error) {
	return c.Get(ctx, "/v2/thesis", p.values())
}

func (c *Client) TokenTheses(ctx context.Context, mint string, p Params) (*Response, error) {
	return c.Get(ctx, "/v2/thesis/token/"+esc(mint), p.values())
}

func (c *Client) UserTheses(ctx context.Context, id, token string, p Params) (*Response, error) {
	path := "/v2/thesis/user/" + esc(id)
	if token != "" {
		path += "/token/" + esc(token)
	}
	return c.Get(ctx, path, p.values())
}

// ---- search & alerts ----

func (c *Client) Search(ctx context.Context, q, typ string, limit int) (*Response, error) {
	return c.Get(ctx, "/v2/search", Params{"q": q, "type": typ, "limit": Itoa(limit)}.values())
}

func (c *Client) TokenSearch(ctx context.Context, q string, limit int) (*Response, error) {
	return c.Get(ctx, "/v2/tokens/search", Params{"q": q, "limit": Itoa(limit)}.values())
}

func (c *Client) Alerts(ctx context.Context, p Params) (*Response, error) {
	return c.Get(ctx, "/v2/alerts", p.values())
}

// Raw performs a GET against any path - the escape hatch behind `fomocli api`.
func (c *Client) Raw(ctx context.Context, path string, q url.Values) (*Response, error) {
	return c.Get(ctx, path, q)
}
