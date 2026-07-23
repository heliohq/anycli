package pipedrive

import (
	"net/http"

	"github.com/spf13/cobra"
)

// Entity command families. Each newXGroup builds a resource (pure data) and
// assembles its cobra group from the generic op builders in resource.go. v2
// entities use cursor pagination + PATCH updates; the v1-only entities (leads,
// notes) use offset pagination, and notes update with PUT.

func (s *Service) newDealGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "deal", short: "Manage deals", path: "/api/v2/deals",
		paginate: paginateCursor,
		filters: []filterFlag{
			{"person-id", "person_id", "filter by linked person id"},
			{"org-id", "org_id", "filter by linked organization id"},
			{"pipeline-id", "pipeline_id", "filter by pipeline id"},
			{"stage-id", "stage_id", "filter by stage id"},
			{"status", "status", "filter by status (open|won|lost|deleted)"},
			{"owner-id", "owner_id", "filter by owner user id"},
		},
		fields: []fieldSpec{
			{"title", "title", fieldString, "deal title"},
			{"value", "value", fieldFloat, "deal monetary value"},
			{"currency", "currency", fieldString, "deal currency (e.g. USD)"},
			{"person-id", "person_id", fieldInt, "linked person id"},
			{"org-id", "org_id", fieldInt, "linked organization id"},
			{"pipeline-id", "pipeline_id", fieldInt, "pipeline id"},
			{"stage-id", "stage_id", fieldInt, "stage id (move the deal)"},
			{"status", "status", fieldString, "status: open|won|lost"},
			{"lost-reason", "lost_reason", fieldString, "reason when status=lost"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.searchCmd())
}

func (s *Service) newPersonGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "person", short: "Manage persons (contacts)", path: "/api/v2/persons",
		paginate: paginateCursor,
		filters: []filterFlag{
			{"org-id", "org_id", "filter by linked organization id"},
			{"owner-id", "owner_id", "filter by owner user id"},
		},
		fields: []fieldSpec{
			{"name", "name", fieldString, "person name"},
			{"org-id", "org_id", fieldInt, "linked organization id"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.searchCmd())
}

func (s *Service) newOrgGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "org", short: "Manage organizations (accounts)", path: "/api/v2/organizations",
		paginate: paginateCursor,
		filters: []filterFlag{
			{"owner-id", "owner_id", "filter by owner user id"},
		},
		fields: []fieldSpec{
			{"name", "name", fieldString, "organization name"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
			{"address", "address", fieldString, "organization address"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.searchCmd())
}

func (s *Service) newActivityGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "activity", short: "Manage activities (calls, meetings, tasks)", path: "/api/v2/activities",
		paginate: paginateCursor,
		filters: []filterFlag{
			{"deal-id", "deal_id", "filter by linked deal id"},
			{"person-id", "person_id", "filter by linked person id"},
			{"org-id", "org_id", "filter by linked organization id"},
			{"owner-id", "owner_id", "filter by owner user id"},
			{"done", "done", "filter by done state (0|1)"},
		},
		fields: []fieldSpec{
			{"subject", "subject", fieldString, "activity subject"},
			{"type", "type", fieldString, "activity type key (e.g. call, meeting, task)"},
			{"due-date", "due_date", fieldString, "due date (YYYY-MM-DD)"},
			{"due-time", "due_time", fieldString, "due time (HH:MM)"},
			{"deal-id", "deal_id", fieldInt, "linked deal id"},
			{"person-id", "person_id", fieldInt, "linked person id"},
			{"org-id", "org_id", fieldInt, "linked organization id"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
			{"done", "done", fieldBool, "mark the activity done"},
			{"note", "note", fieldString, "activity note"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.deleteCmd())
}

func (s *Service) newLeadGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "lead", short: "Manage leads (v1)", path: "/api/v1/leads",
		paginate: paginateOffset, updateMethod: http.MethodPatch,
		filters: []filterFlag{
			{"owner-id", "owner_id", "filter by owner user id"},
			{"person-id", "person_id", "filter by linked person id"},
			{"org-id", "organization_id", "filter by linked organization id"},
			{"archived-status", "archived_status", "filter by archived status (archived|not_archived|all)"},
		},
		fields: []fieldSpec{
			{"title", "title", fieldString, "lead title"},
			{"owner-id", "owner_id", fieldInt, "owner user id"},
			{"person-id", "person_id", fieldInt, "linked person id"},
			{"org-id", "organization_id", fieldInt, "linked organization id"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.deleteCmd())
}

func (s *Service) newNoteGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "note", short: "Manage notes (v1)", path: "/api/v1/notes",
		paginate: paginateOffset, createVerb: "add", updateMethod: http.MethodPut,
		filters: []filterFlag{
			{"deal-id", "deal_id", "filter by deal id"},
			{"person-id", "person_id", "filter by person id"},
			{"org-id", "org_id", "filter by organization id"},
			{"lead-id", "lead_id", "filter by lead id"},
		},
		fields: []fieldSpec{
			{"content", "content", fieldString, "note content (HTML supported)"},
			{"deal-id", "deal_id", fieldInt, "attach to deal id"},
			{"person-id", "person_id", fieldInt, "attach to person id"},
			{"org-id", "org_id", fieldInt, "attach to organization id"},
			{"lead-id", "lead_id", fieldString, "attach to lead id (UUID)"},
		},
	}
	return r.group(r.listCmd(), r.getCmd(), r.createCmd(), r.updateCmd(), r.deleteCmd())
}

func (s *Service) newPipelineGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "pipeline", short: "Read pipelines", path: "/api/v2/pipelines",
		paginate: paginateCursor,
	}
	return r.group(r.listCmd(), r.getCmd())
}

func (s *Service) newStageGroup(c *caller) *cobra.Command {
	r := resource{
		c: c, word: "stage", short: "Read stages", path: "/api/v2/stages",
		paginate: paginateCursor,
		filters: []filterFlag{
			{"pipeline-id", "pipeline_id", "filter stages by pipeline id"},
		},
	}
	return r.group(r.listCmd(), r.getCmd())
}
