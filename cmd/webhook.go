package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/figmaurl"
	"github.com/branow/figma-cli/internal/output"
)

// newWebhookCmd groups the webhook list/create/get/update/delete
// subcommands.
func newWebhookCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Manage team webhooks",
	}
	cmd.AddCommand(
		newWebhookListCmd(f),
		newWebhookCreateCmd(f),
		newWebhookGetCmd(f),
		newWebhookUpdateCmd(f),
		newWebhookDeleteCmd(f),
	)
	return cmd
}

func newWebhookListCmd(f *cmdutil.Factory) *cobra.Command {
	var context, contextID, plan, team string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List webhooks by context, plan, or team",
		Args:    cobra.NoArgs,
		Example: `  figma webhook list --context team --context-id 1234567890
  figma webhook list --plan 5678
  figma webhook list --team 1234567890`,
		RunE: func(cmd *cobra.Command, args []string) error {
			op := "getWebhooks"
			fields := map[string]any{}
			switch {
			case team != "":
				id, ok := figmaurl.TeamID(team)
				if !ok {
					return &cmdutil.ValidationError{Message: "could not parse a team id from " + team}
				}
				op = "getTeamWebhooks"
				fields["team_id"] = id
			case plan != "":
				fields["plan_api_id"] = plan
			case context != "" && contextID != "":
				fields["context"] = context
				fields["context_id"] = contextID
			default:
				return &cmdutil.ValidationError{Message: "provide --team, --plan, or both --context and --context-id"}
			}
			resp, err := callAPI(cmd.Context(), f, op, fields)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return writeResultJSON(f, resp.Raw)
			}
			hooks, err := figma.DecodeArray[figma.Webhook](resp.Raw, "webhooks")
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(hooks))
			for _, h := range hooks {
				rows = append(rows, []string{h.ID, h.EventType, h.Status, h.Endpoint})
			}
			return output.WriteTable(f.IOStreams.Out, []string{"ID", "EVENT", "STATUS", "ENDPOINT"}, rows)
		},
	}
	cmd.Flags().StringVar(&context, "context", "", `context type: "team", "project", or "file"`)
	cmd.Flags().StringVar(&contextID, "context-id", "", "id of the context to list webhooks for")
	cmd.Flags().StringVar(&plan, "plan", "", "plan api id to list all accessible webhooks")
	cmd.Flags().StringVar(&team, "team", "", "team id to list webhooks for")
	return cmd
}

func newWebhookCreateCmd(f *cmdutil.Factory) *cobra.Command {
	var eventType, endpoint, passcode, context, contextID, description, status string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook",
		Args:  cobra.NoArgs,
		Example: `  figma webhook create --event-type FILE_UPDATE --context file \
    --context-id abc123 --endpoint https://example.com/hook --passcode s3cret`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireFlags(
				[2]string{"--event-type", eventType}, [2]string{"--endpoint", endpoint},
				[2]string{"--passcode", passcode}, [2]string{"--context", context},
				[2]string{"--context-id", contextID},
			); err != nil {
				return err
			}
			if err := validateWebhookStatus(status); err != nil {
				return err
			}
			fields := map[string]any{
				"event_type": eventType,
				"endpoint":   endpoint,
				"passcode":   passcode,
				"context":    context,
				"context_id": contextID,
			}
			addOptional(fields, "description", description)
			addOptional(fields, "status", status)
			return mutate(cmd.Context(), f, "postWebhook", fields, func(raw []byte) string {
				if id := decodeID(raw); id != "" {
					return "Created webhook " + id
				}
				return "Created webhook"
			})
		},
	}
	cmd.Flags().StringVar(&eventType, "event-type", "", "event to subscribe to, e.g. FILE_UPDATE")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "HTTPS endpoint to receive events")
	cmd.Flags().StringVar(&passcode, "passcode", "", "passcode echoed back to your endpoint")
	cmd.Flags().StringVar(&context, "context", "", `context type: "team", "project", or "file"`)
	cmd.Flags().StringVar(&contextID, "context-id", "", "id of the context to watch")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	cmd.Flags().StringVar(&status, "status", "", "initial status: ACTIVE or PAUSED")
	return cmd
}

func newWebhookGetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "get <webhook-id>",
		Short:   "Get a webhook by id",
		Args:    cobra.ExactArgs(1),
		Example: `  figma webhook get 424242`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJSONCall(cmd.Context(), f, "getWebhook", map[string]any{"webhook_id": args[0]})
		},
	}
}

func newWebhookUpdateCmd(f *cmdutil.Factory) *cobra.Command {
	var eventType, endpoint, passcode, description, status string
	cmd := &cobra.Command{
		Use:   "update <webhook-id>",
		Short: "Update a webhook",
		Args:  cobra.ExactArgs(1),
		Example: `  figma webhook update 424242 --status PAUSED
  figma webhook update 424242 --endpoint https://example.com/new`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateWebhookStatus(status); err != nil {
				return err
			}
			fields := map[string]any{"webhook_id": args[0]}
			addOptional(fields, "event_type", eventType)
			addOptional(fields, "endpoint", endpoint)
			addOptional(fields, "passcode", passcode)
			addOptional(fields, "description", description)
			addOptional(fields, "status", status)
			return mutate(cmd.Context(), f, "putWebhook", fields, func(raw []byte) string {
				return "Updated webhook " + args[0]
			})
		},
	}
	cmd.Flags().StringVar(&eventType, "event-type", "", "event to subscribe to")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "HTTPS endpoint to receive events")
	cmd.Flags().StringVar(&passcode, "passcode", "", "passcode echoed back to your endpoint")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	cmd.Flags().StringVar(&status, "status", "", "status: ACTIVE or PAUSED")
	return cmd
}

func newWebhookDeleteCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <webhook-id>",
		Short:   "Delete a webhook",
		Args:    cobra.ExactArgs(1),
		Example: `  figma webhook delete 424242`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := callAPI(cmd.Context(), f, "deleteWebhook", map[string]any{"webhook_id": args[0]})
			if err != nil {
				return err
			}
			if !f.Quiet {
				fmt.Fprintf(f.IOStreams.Out, "Deleted webhook %s\n", args[0])
			}
			return nil
		},
	}
}
