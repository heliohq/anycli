package contacts

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// searchOutcome carries one endpoint's parallel search result.
type searchOutcome struct {
	people []person
	err    error
}

// resolveMatch is one merged, source-tagged candidate in resolve's output.
type resolveMatch struct {
	ResourceName string   `json:"resourceName"`
	Name         string   `json:"name"`
	Emails       []string `json:"emails"`
	Phones       []string `json:"phones"`
	Organization string   `json:"organization,omitempty"`
	Source       string   `json:"source"` // "my_contact" | "other_contact"
}

// resolveOutput is resolve's synthesized --json envelope.
type resolveOutput struct {
	Query   string         `json:"query"`
	Count   int            `json:"count"`
	Matches []resolveMatch `json:"matches"`
}

const (
	sourceMyContact    = "my_contact"
	sourceOtherContact = "other_contact"
)

func (s *Service) newResolveCmd(token string) *cobra.Command {
	var max int
	cmd := &cobra.Command{
		Use:   "resolve <name>",
		Short: "Resolve a name to email/phone across My Contacts + Other Contacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			matches, err := s.resolveName(cmd.Context(), token, name, max)
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				// A zero-hit lookup is a successful query, not an error, but
				// Execute maps resolveMiss to a distinct exit code so the
				// caller can tell it apart.
				s.resolveMiss = true
				if jsonOut(cmd) {
					return s.emitJSON(resolveOutput{Query: name, Count: 0, Matches: []resolveMatch{}})
				}
				fmt.Fprintf(s.stdout(), "no contacts matched %q\n", name)
				return nil
			}
			if jsonOut(cmd) {
				return s.emitJSON(resolveOutput{Query: name, Count: len(matches), Matches: matches})
			}
			for _, m := range matches {
				renderMatch(s.stdout(), m)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&max, "max", 10, "max results per source (API hard cap 30)")
	return cmd
}

// resolveName runs both search endpoints in parallel, then merges the results:
// My Contacts first, Other Contacts appended, de-duplicated by primary email
// (falling back to resource name). A failure on either endpoint is surfaced —
// no silent partial result.
func (s *Service) resolveName(ctx context.Context, token, name string, max int) ([]resolveMatch, error) {
	myCh := make(chan searchOutcome, 1)
	otherCh := make(chan searchOutcome, 1)
	go func() {
		_, people, err := s.searchOnce(ctx, token, "/people:searchContacts", name, defaultPersonFields, max)
		myCh <- searchOutcome{people: people, err: err}
	}()
	go func() {
		_, people, err := s.searchOnce(ctx, token, "/otherContacts:search", name, otherReadMask, max)
		otherCh <- searchOutcome{people: people, err: err}
	}()
	my := <-myCh
	other := <-otherCh
	if my.err != nil {
		return nil, my.err
	}
	if other.err != nil {
		return nil, other.err
	}

	seen := make(map[string]bool)
	matches := make([]resolveMatch, 0, len(my.people)+len(other.people))
	add := func(people []person, source string) {
		for i := range people {
			p := &people[i]
			key := p.dedupKey()
			if seen[key] {
				continue
			}
			seen[key] = true
			matches = append(matches, resolveMatch{
				ResourceName: p.ResourceName,
				Name:         p.displayName(),
				Emails:       p.emails(),
				Phones:       p.phones(),
				Organization: p.organization(),
				Source:       source,
			})
		}
	}
	add(my.people, sourceMyContact)
	add(other.people, sourceOtherContact)
	return matches, nil
}

// renderMatch prints one resolve candidate in the human-readable view.
func renderMatch(w io.Writer, m resolveMatch) {
	fmt.Fprintf(w, "%s [%s]\n", m.Name, m.Source)
	for _, e := range m.Emails {
		fmt.Fprintf(w, "  email: %s\n", e)
	}
	for _, p := range m.Phones {
		fmt.Fprintf(w, "  phone: %s\n", p)
	}
	if m.Organization != "" {
		fmt.Fprintf(w, "  org:   %s\n", m.Organization)
	}
}
