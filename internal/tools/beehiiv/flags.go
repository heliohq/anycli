package beehiiv

import "github.com/spf13/cobra"

// addPublicationFlag registers the required --publication-id flag on a leaf
// command. Almost every beehiiv resource is publication-scoped; the flag is
// required so a missing value is a clean usage error (exit 2) rather than a
// path built against an empty id.
func addPublicationFlag(cmd *cobra.Command) *string {
	pub := cmd.Flags().String("publication-id", "", "beehiiv publication id (pub_…); run `publication list` to find it")
	_ = cmd.MarkFlagRequired("publication-id")
	return pub
}
