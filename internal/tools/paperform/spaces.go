package paperform

import (
	"net/url"

	"github.com/spf13/cobra"
)

// newSpaceCmd builds the `space` group: list spaces, get one, and list a
// space's forms — the workspace-tree navigation surface.
func (s *Service) newSpaceCmd(key string) *cobra.Command {
	group := newGroupCmd("space", "Navigate the workspace (spaces)")

	list := &cobra.Command{
		Use:   "list",
		Short: "List spaces",
		Long: "Spaces are the folders an account's forms are organized into. No flags and\n" +
			"no paging — every space the key can see comes back at once. Descend into\n" +
			"one with `space forms --id`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runGet(cmd, key, "/spaces", nil)
		},
	}

	var getID string
	get := &cobra.Command{
		Use:   "get",
		Short: "Get a single space by ID",
		Long: "`--id` is required and is the space id from `space list`; unlike a form, a\n" +
			"space is not addressable by slug. Returns the space itself, not the forms\n" +
			"inside it — that is `space forms`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if getID == "" {
				return &usageError{msg: "space get: --id is required"}
			}
			return s.runGet(cmd, key, "/spaces/"+url.PathEscape(getID), nil)
		},
	}
	get.Flags().StringVar(&getID, "id", "", "space ID (required)")

	var formsID string
	forms := &cobra.Command{
		Use:   "forms",
		Short: "List the forms in a space",
		Long: "`--id` is the space id. This is the way to scope a large account to one\n" +
			"area of work instead of paging the flat `form list`. It takes no paging\n" +
			"flags of its own, so a very large space returns whatever Paperform\n" +
			"defaults to.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if formsID == "" {
				return &usageError{msg: "space forms: --id is required"}
			}
			return s.runGet(cmd, key, "/spaces/"+url.PathEscape(formsID)+"/forms", nil)
		},
	}
	forms.Flags().StringVar(&formsID, "id", "", "space ID (required)")

	group.AddCommand(list, get, forms)
	return group
}

// newProductCmd builds the `product` group: list a form's products.
func (s *Service) newProductCmd(key string) *cobra.Command {
	group := newGroupCmd("product", "Read a form's product/order config")

	var formID string
	list := &cobra.Command{
		Use:   "list",
		Short: "List a form's products",
		Long: "The purchasable items a commerce-enabled form sells, with their prices and\n" +
			"currency. Needed to make sense of an order-carrying submission, whose\n" +
			"answers reference products rather than restating their prices. `--form` is\n" +
			"required; a form that sells nothing returns an empty set rather than an\n" +
			"error.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if formID == "" {
				return &usageError{msg: "product list: --form is required"}
			}
			return s.runGet(cmd, key, "/forms/"+url.PathEscape(formID)+"/products", nil)
		},
	}
	list.Flags().StringVar(&formID, "form", "", "form slug or ID (required)")

	group.AddCommand(list)
	return group
}

// newCouponCmd builds the `coupon` group: list a form's coupons and get one by
// code.
func (s *Service) newCouponCmd(key string) *cobra.Command {
	group := newGroupCmd("coupon", "Read a form's discount coupons")

	var listFormID string
	list := &cobra.Command{
		Use:   "list",
		Short: "List a form's coupons",
		Long: "Coupons are per-form, so there is no account-wide coupon view: ask each form\n" +
			"separately. Each carries its code, discount type and value, which is what\n" +
			"explains a submission total that is lower than the sum of its products.\n" +
			"`--form` is required.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if listFormID == "" {
				return &usageError{msg: "coupon list: --form is required"}
			}
			return s.runGet(cmd, key, "/forms/"+url.PathEscape(listFormID)+"/coupons", nil)
		},
	}
	list.Flags().StringVar(&listFormID, "form", "", "form slug or ID (required)")

	var getFormID, code string
	get := &cobra.Command{
		Use:   "get",
		Short: "Get a single coupon by code",
		Long: "`--code` is the coupon CODE a respondent types, not an id, and `--form`\n" +
			"scopes it because the same code can exist on more than one form. Both are\n" +
			"required.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if getFormID == "" {
				return &usageError{msg: "coupon get: --form is required"}
			}
			if code == "" {
				return &usageError{msg: "coupon get: --code is required"}
			}
			return s.runGet(cmd, key, "/forms/"+url.PathEscape(getFormID)+"/coupons/"+url.PathEscape(code), nil)
		},
	}
	get.Flags().StringVar(&getFormID, "form", "", "form slug or ID (required)")
	get.Flags().StringVar(&code, "code", "", "coupon code (required)")

	group.AddCommand(list, get)
	return group
}
