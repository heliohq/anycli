package stripe

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// listOpts holds the cursor-pagination flags Stripe list endpoints accept plus
// a repeatable --param passthrough for list filters (e.g. customer=cus_123).
type listOpts struct {
	limit         int
	startingAfter string
	endingBefore  string
	params        []string
}

// registerListFlags wires --limit / --starting-after / --ending-before /
// --param onto a list command. limit 0 = omit (Stripe defaults to 10).
func registerListFlags(cmd *cobra.Command, o *listOpts) {
	cmd.Flags().IntVar(&o.limit, "limit", 0, "page size 1-100 (Stripe default 10 when omitted)")
	cmd.Flags().StringVar(&o.startingAfter, "starting-after", "", "cursor: object id to page forward from")
	cmd.Flags().StringVar(&o.endingBefore, "ending-before", "", "cursor: object id to page backward from")
	cmd.Flags().StringArrayVar(&o.params, "param", nil, "extra list filter key=value (repeatable, e.g. --param customer=cus_123)")
}

// query builds the URL query for a list request from the pagination flags and
// any --param filters.
func (o listOpts) query() (url.Values, error) {
	q := url.Values{}
	if o.limit != 0 {
		if o.limit < 1 || o.limit > 100 {
			return nil, &usageError{msg: fmt.Sprintf("stripe: --limit must be 1-100, got %d", o.limit)}
		}
		q.Set("limit", strconv.Itoa(o.limit))
	}
	if o.startingAfter != "" {
		q.Set("starting_after", o.startingAfter)
	}
	if o.endingBefore != "" {
		q.Set("ending_before", o.endingBefore)
	}
	if err := applyParams(q, o.params); err != nil {
		return nil, err
	}
	return q, nil
}

// mutOpts holds the shared flags every create/update/delete verb accepts: a
// repeatable --param that maps 1:1 onto Stripe's form fields (bracket notation
// like metadata[order]=1 passes through verbatim) plus --idempotency-key.
type mutOpts struct {
	params         []string
	idempotencyKey string
}

// registerMutationFlags wires --param / --idempotency-key onto a mutation
// command.
func registerMutationFlags(cmd *cobra.Command, o *mutOpts) {
	cmd.Flags().StringArrayVar(&o.params, "param", nil, "request field key=value (repeatable, e.g. --param amount=1000 --param currency=usd)")
	cmd.Flags().StringVar(&o.idempotencyKey, "idempotency-key", "", "Idempotency-Key header for safe retries")
}

// form builds the application/x-www-form-urlencoded body from --param entries.
// It always returns a non-nil url.Values (possibly empty) so the caller sends a
// form body — Stripe accepts an empty body for parameterless mutations like
// invoice finalize.
func (o mutOpts) form() (url.Values, error) {
	f := url.Values{}
	if err := applyParams(f, o.params); err != nil {
		return nil, err
	}
	return f, nil
}

// applyParams parses repeatable "key=value" entries (split on the first "=")
// into the given values set. A missing "=" or empty key is a usage error.
func applyParams(v url.Values, params []string) error {
	for _, p := range params {
		key, value, ok := strings.Cut(p, "=")
		if !ok || key == "" {
			return &usageError{msg: fmt.Sprintf("stripe: --param must be key=value, got %q", p)}
		}
		v.Add(key, value)
	}
	return nil
}

// newListGetGroup builds a resource group exposing the two universal
// read verbs Stripe gives every list-shaped resource: `list` (GET
// <basePath>, cursor-paginated) and `get <id>` (GET <basePath>/<id>).
func (s *Service) newListGetGroup(token, use, short, basePath string) *cobra.Command {
	group := newGroupCmd(use, short)
	group.AddCommand(
		s.newListCmd(token, basePath),
		s.newGetByIDCmd(token, basePath),
	)
	return group
}

// newListCmd builds a paginated `list` subcommand for basePath.
func (s *Service) newListCmd(token, basePath string) *cobra.Command {
	var o listOpts
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + strings.TrimPrefix(basePath, "/"),
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := o.query()
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, basePath, callOpts{query: q})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, &o)
	return cmd
}

// newGetByIDCmd builds a `get <id>` subcommand for basePath (retrieve one).
func (s *Service) newGetByIDCmd(token, basePath string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Retrieve one " + strings.TrimSuffix(strings.TrimPrefix(basePath, "/"), "s") + " by id",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, basePath+"/"+url.PathEscape(args[0]), callOpts{})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
