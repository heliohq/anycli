package outreach

import (
	"net/url"

	"github.com/spf13/cobra"
)

var prospectResource = resource{path: "prospects", typ: "prospect"}

// newProspectCmd builds the prospect resource group (the core CRM object).
func (s *Service) newProspectCmd(token string) *cobra.Command {
	group := newGroupCmd("prospect", "Look up and maintain prospects")
	group.AddCommand(
		s.newProspectListCmd(token),
		s.newGetCmd(token, prospectResource),
		s.newProspectCreateCmd(token),
		s.newProspectUpdateCmd(token),
	)
	return group
}

func (s *Service) newProspectListCmd(token string) *cobra.Command {
	var q, email, accountID, stageID, ownerID string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List prospects (one page)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setFilter(query, "q", q)                  // documented global/full-text filter (prospects only)
			setFilter(query, "emails", email)         // field filter
			setRelFilter(query, "account", accountID) // filter[account][id]
			setRelFilter(query, "stage", stageID)     // filter[stage][id]
			setRelFilter(query, "owner", ownerID)     // filter[owner][id]
			if err := listFlagsFrom(cmd).apply(query, prospectResource.typ); err != nil {
				return err
			}
			return s.runList(cmd.Context(), token, prospectResource, query)
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "global full-text search over prospects (filter[q])")
	cmd.Flags().StringVar(&email, "email", "", "filter by email address (filter[emails])")
	cmd.Flags().StringVar(&accountID, "account-id", "", "filter by account id")
	cmd.Flags().StringVar(&stageID, "stage-id", "", "filter by stage id")
	cmd.Flags().StringVar(&ownerID, "owner-id", "", "filter by owner (user) id")
	bindListFlags(cmd)
	return cmd
}

func (s *Service) newProspectCreateCmd(token string) *cobra.Command {
	f := &prospectFields{}
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a prospect",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			attrs, rels, err := f.build()
			if err != nil {
				return err
			}
			return s.runCreate(cmd.Context(), token, prospectResource, attrs, rels)
		},
	}
	f.bind(cmd)
	return cmd
}

func (s *Service) newProspectUpdateCmd(token string) *cobra.Command {
	f := &prospectFields{}
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a prospect",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			attrs, rels, err := f.build()
			if err != nil {
				return err
			}
			return s.runUpdate(cmd.Context(), token, prospectResource, args[0], attrs, rels)
		},
	}
	f.bind(cmd)
	return cmd
}

// prospectFields collects the convenience + generic attribute/relationship flags
// shared by prospect create and update.
type prospectFields struct {
	firstName string
	lastName  string
	title     string
	email     string
	accountID string
	ownerID   string
	stageID   string
	attr      []string
}

func (f *prospectFields) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.firstName, "first-name", "", "prospect first name")
	cmd.Flags().StringVar(&f.lastName, "last-name", "", "prospect last name")
	cmd.Flags().StringVar(&f.title, "title", "", "prospect job title")
	cmd.Flags().StringVar(&f.email, "email", "", "prospect email address (sets the emails attribute)")
	cmd.Flags().StringVar(&f.accountID, "account-id", "", "related account id")
	cmd.Flags().StringVar(&f.ownerID, "owner-id", "", "owner (user) id")
	cmd.Flags().StringVar(&f.stageID, "stage-id", "", "stage id")
	cmd.Flags().StringArrayVar(&f.attr, "attr", nil, "additional attribute key=value (repeatable; value parsed as JSON when valid)")
}

func (f *prospectFields) build() (map[string]any, map[string]string, error) {
	attrs, err := parseAttrs(f.attr)
	if err != nil {
		return nil, nil, err
	}
	setAttr(attrs, "firstName", f.firstName)
	setAttr(attrs, "lastName", f.lastName)
	setAttr(attrs, "title", f.title)
	if f.email != "" {
		attrs["emails"] = []string{f.email}
	}
	rels := map[string]string{}
	setRel(rels, "account", f.accountID)
	setRel(rels, "owner", f.ownerID)
	setRel(rels, "stage", f.stageID)
	return attrs, rels, nil
}

var accountResource = resource{path: "accounts", typ: "account"}

// newAccountCmd builds the account resource group (company context).
func (s *Service) newAccountCmd(token string) *cobra.Command {
	group := newGroupCmd("account", "Look up and maintain accounts")
	group.AddCommand(
		s.newAccountListCmd(token),
		s.newGetCmd(token, accountResource),
		s.newAccountCreateCmd(token),
		s.newAccountUpdateCmd(token),
	)
	return group
}

func (s *Service) newAccountListCmd(token string) *cobra.Command {
	var q, domain, name, ownerID string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List accounts (one page)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setFilter(query, "q", q)              // documented global/full-text filter (accounts only)
			setFilter(query, "domain", domain)    // field filter
			setFilter(query, "name", name)        // field filter
			setRelFilter(query, "owner", ownerID) // filter[owner][id]
			if err := listFlagsFrom(cmd).apply(query, accountResource.typ); err != nil {
				return err
			}
			return s.runList(cmd.Context(), token, accountResource, query)
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "global full-text search over accounts (filter[q])")
	cmd.Flags().StringVar(&domain, "domain", "", "filter by company domain")
	cmd.Flags().StringVar(&name, "name", "", "filter by account name")
	cmd.Flags().StringVar(&ownerID, "owner-id", "", "filter by owner (user) id")
	bindListFlags(cmd)
	return cmd
}

func (s *Service) newAccountCreateCmd(token string) *cobra.Command {
	f := &accountFields{}
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create an account",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			attrs, rels, err := f.build()
			if err != nil {
				return err
			}
			return s.runCreate(cmd.Context(), token, accountResource, attrs, rels)
		},
	}
	f.bind(cmd)
	return cmd
}

func (s *Service) newAccountUpdateCmd(token string) *cobra.Command {
	f := &accountFields{}
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update an account",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			attrs, rels, err := f.build()
			if err != nil {
				return err
			}
			return s.runUpdate(cmd.Context(), token, accountResource, args[0], attrs, rels)
		},
	}
	f.bind(cmd)
	return cmd
}

// accountFields collects the flags shared by account create and update.
type accountFields struct {
	name    string
	domain  string
	ownerID string
	attr    []string
}

func (f *accountFields) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.name, "name", "", "account name")
	cmd.Flags().StringVar(&f.domain, "domain", "", "company domain")
	cmd.Flags().StringVar(&f.ownerID, "owner-id", "", "owner (user) id")
	cmd.Flags().StringArrayVar(&f.attr, "attr", nil, "additional attribute key=value (repeatable; value parsed as JSON when valid)")
}

func (f *accountFields) build() (map[string]any, map[string]string, error) {
	attrs, err := parseAttrs(f.attr)
	if err != nil {
		return nil, nil, err
	}
	setAttr(attrs, "name", f.name)
	setAttr(attrs, "domain", f.domain)
	rels := map[string]string{}
	setRel(rels, "owner", f.ownerID)
	return attrs, rels, nil
}
