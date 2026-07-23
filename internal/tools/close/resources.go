package close

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newResourceCmd builds the standard CRUD command group shared by lead,
// contact, and opportunity: list / get / create / update / delete over a
// Close collection path (e.g. "/lead/"). The item path is collection+id+"/".
func (s *Service) newResourceCmd(token, name, collectionPath, short string) *cobra.Command {
	group := newGroupCmd(name, short)
	group.AddCommand(
		s.newListCmd(token, name, collectionPath),
		s.newGetCmd(token, name, collectionPath),
		s.newCreateCmd(token, name, collectionPath),
		s.newUpdateCmd(token, name, collectionPath),
		s.newDeleteCmd(token, name, collectionPath),
	)
	return group
}

// listFlags carries the offset-pagination flags Close list endpoints accept.
type listFlags struct {
	limit int
	skip  int
}

func registerListFlags(cmd *cobra.Command, lf *listFlags) {
	cmd.Flags().IntVar(&lf.limit, "limit", 0, "max results to return (Close _limit; 0 = provider default)")
	cmd.Flags().IntVar(&lf.skip, "skip", 0, "results to skip for pagination (Close _skip)")
}

// apply writes the pagination flags into a query value set, omitting unset ones.
func (lf listFlags) apply(q url.Values) {
	if lf.limit > 0 {
		q.Set("_limit", strconv.Itoa(lf.limit))
	}
	if lf.skip > 0 {
		q.Set("_skip", strconv.Itoa(lf.skip))
	}
}

func (s *Service) newListCmd(token, name, collectionPath string) *cobra.Command {
	var lf listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + name + "s (paginated)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			lf.apply(q)
			body, err := s.call(cmd.Context(), token, http.MethodGet, collectionPath, q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, &lf)
	return cmd
}

func (s *Service) newGetCmd(token, name, collectionPath string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one " + name + " by id",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, collectionPath+url.PathEscape(args[0])+"/", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newCreateCmd(token, name, collectionPath string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create --data <json|@file>",
		Short:       "Create a " + name + " from a JSON body",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := readData("data", data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, collectionPath, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON object (or @file.json) for the new "+name)
	return cmd
}

func (s *Service) newUpdateCmd(token, name, collectionPath string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "update <id> --data <json|@file>",
		Short:       "Update a " + name + " from a JSON body",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readData("data", data)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut, collectionPath+url.PathEscape(args[0])+"/", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON object (or @file.json) of fields to update")
	return cmd
}

func (s *Service) newDeleteCmd(token, name, collectionPath string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a " + name + " by id",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodDelete, collectionPath+url.PathEscape(args[0])+"/", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newTaskCmd is the task resource: the standard CRUD group plus `complete`,
// which PUTs {"is_complete": true} onto a task.
func (s *Service) newTaskCmd(token string) *cobra.Command {
	group := s.newResourceCmd(token, "task", "/task/", "Manage tasks (follow-up reminders)")
	group.AddCommand(&cobra.Command{
		Use:         "complete <id>",
		Short:       "Mark a task complete",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/task/"+url.PathEscape(args[0])+"/", nil, map[string]any{"is_complete": true})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	})
	return group
}
