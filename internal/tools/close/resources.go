package close

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// resourceLongs carries the five CRUD Longs for one Close resource. Lead,
// contact, opportunity and task are all built by the one shared constructor
// below, but they differ in what their --data body must carry and in what a
// delete destroys, so each resource declares its own prose here rather than
// sharing one generic text.
type resourceLongs struct {
	list   string
	get    string
	create string
	update string
	delete string
}

// newResourceCmd builds the standard CRUD command group shared by lead,
// contact, and opportunity: list / get / create / update / delete over a
// Close collection path (e.g. "/lead/"). The item path is collection+id+"/".
func (s *Service) newResourceCmd(token, name, collectionPath, short string, longs resourceLongs) *cobra.Command {
	group := newGroupCmd(name, short)
	group.AddCommand(
		s.newListCmd(token, name, collectionPath, longs.list),
		s.newGetCmd(token, name, collectionPath, longs.get),
		s.newCreateCmd(token, name, collectionPath, longs.create),
		s.newUpdateCmd(token, name, collectionPath, longs.update),
		s.newDeleteCmd(token, name, collectionPath, longs.delete),
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

func (s *Service) newListCmd(token, name, collectionPath, long string) *cobra.Command {
	var lf listFlags
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + name + "s (paginated)",
		Long:        long,
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

func (s *Service) newGetCmd(token, name, collectionPath, long string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one " + name + " by id",
		Long:        long,
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

func (s *Service) newCreateCmd(token, name, collectionPath, long string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create --data <json|@file>",
		Short:       "Create a " + name + " from a JSON body",
		Long:        long,
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

func (s *Service) newUpdateCmd(token, name, collectionPath, long string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "update <id> --data <json|@file>",
		Short:       "Update a " + name + " from a JSON body",
		Long:        long,
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

func (s *Service) newDeleteCmd(token, name, collectionPath, long string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a " + name + " by id",
		Long:        long,
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
	group := s.newResourceCmd(token, "task", "/task/", "Manage tasks (follow-up reminders)", taskLongs)
	group.AddCommand(&cobra.Command{
		Use:   "complete <id>",
		Short: "Mark a task complete",
		Long: "Sends is_complete=true and NOTHING else — it cannot reassign the task,\n" +
			"move its due date or edit its text, and it is not a toggle. Reopening a\n" +
			"task or changing any other field is `task update <id> --data`.",
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

// leadLongs, contactLongs, opportunityLongs and taskLongs are the per-resource
// CRUD Longs. They live beside newResourceCmd because that constructor is what
// fixes the --data body contract and the id argument each one describes.
var (
	leadLongs = resourceLongs{
		list: "A flat page of leads in Close's own order — it cannot filter at all.\n" +
			"Finding a particular company by name, status, owner or any field is\n" +
			"`search`, not this. Page with --limit and --skip, continuing while the\n" +
			"response's `has_more` is true.",
		get: "Returns the lead with its contacts and its opportunities embedded, so this\n" +
			"one call usually replaces separate `contact list` and `opportunity list`\n" +
			"reads. What it does NOT include is the interaction history — notes, calls\n" +
			"and emails live in `activity list --lead-id <lead-id>`.",
		create: "--data is the Close lead payload: `name`, `url`, `status_id`, `addresses`,\n" +
			"and per-org `custom.<field_id>` keys, which have no flags and can only be\n" +
			"set through this body. Contacts may be nested in the same payload or added\n" +
			"afterwards with `contact create`. Status values are per-org ids, not names,\n" +
			"so copy one off an existing lead rather than inventing it.",
		update: "A PUT that patches only the keys present in --data; fields not mentioned\n" +
			"survive. Moving a lead through the pipeline means writing `status_id`,\n" +
			"whose values are per-org ids readable off an existing lead. Custom fields\n" +
			"are set the same way, as `custom.<field_id>` keys.",
		delete: "Permanent, with no undo, and it does not stop at the lead: Close removes\n" +
			"the contacts, opportunities and logged activities attached to it as well.\n" +
			"For a lead that is merely dead, writing a lost or disqualified `status_id`\n" +
			"through `lead update` keeps the history.",
	}

	contactLongs = resourceLongs{
		list: "Every contact in the organization, across all leads, in a flat page — it\n" +
			"takes no lead filter. The people on one company come embedded in\n" +
			"`lead get`, which is one call instead of paging this.",
		get: "One person by contact id. The lead they belong to is on the record as\n" +
			"`lead_id`; the reverse direction, all people at a company, is `lead get`.",
		create: "--data must carry `lead_id` — a contact cannot exist without a lead, so\n" +
			"create or find the company first. Emails and phones are arrays of objects\n" +
			"rather than bare strings: `\"emails\":[{\"email\":\"jane@x.com\",\"type\":\"office\"}]`.\n" +
			"Per-org `custom.<field_id>` keys go in the same body.",
		update: "A PUT that patches the keys given in --data. The array fields are the trap:\n" +
			"writing `emails` or `phones` REPLACES the whole array, so adding one\n" +
			"address means reading the contact first and sending every entry back.",
		delete: "Permanent and not undoable. The lead and its opportunities survive; only\n" +
			"this person and their contact details go. Activities already logged against\n" +
			"the lead are not removed with them.",
	}

	opportunityLongs = resourceLongs{
		list: "A flat page of deals across every lead, with no filter by status, owner or\n" +
			"value. Pipeline questions — what is open, what closes this quarter — are\n" +
			"`search` queries. The deals on one company come embedded in `lead get`.",
		get: "One deal by id, including its status, value and the lead it belongs to.",
		create: "--data needs `lead_id` and a `status_id` drawn from the organization's own\n" +
			"pipeline; status ids are per-org and cannot be guessed from a stage name.\n" +
			"`value` is an integer and `value_period` is one_time, monthly or annual —\n" +
			"read an existing deal with `opportunity get` first to confirm the unit that\n" +
			"organization stores values in.",
		update: "A PUT that patches the keys in --data. Advancing a deal is a write to\n" +
			"`status_id`, and marking it won or lost is the same operation with the\n" +
			"organization's won/lost status id — there is no separate close verb.",
		delete: "Permanent, and it erases the deal from every pipeline report that counted\n" +
			"it. A deal that did not close should usually be moved to the lost status\n" +
			"with `opportunity update` instead, which keeps the history.",
	}

	taskLongs = resourceLongs{
		list: "A flat page of tasks with no filter flags at all — not by lead, assignee,\n" +
			"due date or completion state. \"What is due today\" and \"what is still open\"\n" +
			"are `search` queries. Page with --limit and --skip.",
		get: "One task by id, including its lead, assignee, due date and completion state.",
		create: "--data carries `lead_id`, the `text` shown in the task list, a `date` for\n" +
			"when it is due and `assigned_to` for whose queue it lands in. A task is\n" +
			"visible to the whole Close team, so it reads as a commitment made on\n" +
			"someone's behalf.",
		update: "A PUT that patches the keys in --data — the way to reassign a task, move\n" +
			"its due date or rewrite its text. Also the only way to reopen one, since\n" +
			"`task complete` sets completion in one direction only.",
		delete: "Permanent. Finishing work should normally be `task complete`, which keeps\n" +
			"the record of what was done; deleting removes the evidence that the task\n" +
			"ever existed.",
	}
)
