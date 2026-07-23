package buffer

import (
	"github.com/spf13/cobra"
)

// channelsQuery lists the channels under one organization. Selected fields are
// limited to those the official data-model doc demonstrates (id, name,
// service) to stay fail-fast against unverified schema fields.
const channelsQuery = `query($input: ChannelsInput!) { channels(input: $input) { id name service } }`

// channel is the provider-neutral projection of one connected channel.
type channel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Service string `json:"service"`
}

func (s *Service) newChannelListCmd(token string) *cobra.Command {
	var org string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List channels connected to an organization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if org == "" {
				return &usageError{msg: "--org is required"}
			}
			data, err := s.gql(cmd.Context(), token, channelsQuery, map[string]any{
				"input": map[string]any{"organizationId": org},
			})
			if err != nil {
				return err
			}
			var channels []channel
			if err := decodeField(data, "channels", &channels); err != nil {
				return err
			}
			if channels == nil {
				channels = []channel{}
			}
			return s.emitValue(map[string]any{"channels": channels})
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "organization (workspace) id (required)")
	return cmd
}
