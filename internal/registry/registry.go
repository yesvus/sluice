package registry

// Kind is how an account authenticates to its provider.
type Kind string

const (
	KindAPIKey               Kind = "api_key"
	KindFreeTier             Kind = "free_tier"
	KindSubscriptionObserved Kind = "subscription_observed"
)

// Tier is the cost tier of an account.
type Tier string

const (
	TierFree     Tier = "free"
	TierCheap    Tier = "cheap"
	TierFrontier Tier = "frontier"
)

// Account is one configured account.
type Account struct {
	ID          string
	Provider    string
	OnwatchKey  string
	Kind        Kind
	Tier        Tier
	Deployments []string // LiteLLM deployment ids; empty for subscription_observed
	Threshold   float64  // 0 means default 0.9
}

// Registry holds configured accounts.
type Registry struct {
	accounts []Account
}

// New returns a registry holding a copy of accounts; Load validates configuration.
func New(accounts []Account) *Registry {
	return &Registry{accounts: cloneAccounts(accounts)}
}

// Lookup returns the account matching provider and onwatchKey.
func (r *Registry) Lookup(provider, onwatchKey string) (Account, bool) {
	for _, account := range r.accounts {
		if account.Provider == provider && account.OnwatchKey == onwatchKey {
			return cloneAccount(account), true
		}
	}
	return Account{}, false
}

// Accounts returns a copy of every configured account.
func (r *Registry) Accounts() []Account {
	return cloneAccounts(r.accounts)
}

func cloneAccounts(accounts []Account) []Account {
	out := make([]Account, len(accounts))
	for i, account := range accounts {
		out[i] = cloneAccount(account)
	}
	return out
}

func cloneAccount(account Account) Account {
	account.Deployments = append([]string(nil), account.Deployments...)
	return account
}
