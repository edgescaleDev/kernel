package kernel

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// buildRootCommand constructs the Cobra command tree for the kernel.
// Called by Execute() — consumers don't interact with this directly.
func (k *Kernel) buildRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "kernel",
		Short: "Kernel CLI — manage modules, migrations, and organizations",
		// Silence Cobra's default usage/error printing — we handle it via slog.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		k.serveCommand(),
		k.migrateCommand(),
		k.moduleCommand(),
		k.orgCommand(),
	)

	// Add any custom commands registered by the consumer.
	if len(k.customCommands) > 0 {
		root.AddCommand(k.customCommands...)
	}

	return root
}

// ── serve ────────────────────────────────────────────────────────────────────

func (k *Kernel) serveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Boot the kernel and start the HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			// Graceful shutdown on SIGINT / SIGTERM.
			go func() {
				quit := make(chan os.Signal, 1)
				signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
				<-quit
				k.logger.Info("shutdown signal received")
				k.Shutdown(context.Background())
			}()

			return k.Serve()
		},
	}
}

// ── migrate ──────────────────────────────────────────────────────────────────

func (k *Kernel) migrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run all pending database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}
			return k.Migrate()
		},
	}

	cmd.AddCommand(k.migrateStatusCommand())
	return cmd
}

func (k *Kernel) migrateStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show migration status for all modules",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			var applied []SchemaMigration
			if err := k.db.Order("module_id, version ASC").Find(&applied).Error; err != nil {
				return fmt.Errorf("query migrations: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "MODULE\tVERSION\tFILENAME\tAPPLIED AT")
			fmt.Fprintln(w, "──────\t───────\t────────\t──────────")
			for _, m := range applied {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
					m.ModuleID, m.Version, m.Filename, m.AppliedAt.Format("2006-01-02 15:04:05"))
			}
			return w.Flush()
		},
	}
}

// ── module ───────────────────────────────────────────────────────────────────

func (k *Kernel) moduleCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "module",
		Short: "Manage registered modules",
	}

	cmd.AddCommand(
		k.moduleListCommand(),
		k.moduleEnableCommand(),
		k.moduleDisableCommand(),
		k.moduleStatusCommand(),
		k.moduleDepsCommand(),
	)

	return cmd
}

func (k *Kernel) moduleListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered modules",
		RunE: func(cmd *cobra.Command, args []string) error {
			// No Boot needed — modules are registered at compile time.
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tVERSION\tTYPE\tSCHEMA")
			fmt.Fprintln(w, "──\t────\t───────\t────\t──────")
			for _, m := range k.Modules() {
				manifest := m.Manifest()
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					manifest.ID, manifest.Name, manifest.Version,
					manifest.Type.String(), manifest.Schema)
			}
			return w.Flush()
		},
	}
}

func (k *Kernel) moduleEnableCommand() *cobra.Command {
	var orgID string

	cmd := &cobra.Command{
		Use:   "enable [module-id]",
		Short: "Enable a module for an organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			parsedOrg, err := uuid.Parse(orgID)
			if err != nil {
				return fmt.Errorf("invalid org ID: %w", err)
			}

			moduleID := args[0]
			if _, exists := k.manifests[moduleID]; !exists {
				return fmt.Errorf("module %q not registered", moduleID)
			}

			// Use a zero UUID for activated_by in CLI context.
			if err := k.ActivateModule(context.Background(), moduleID, parsedOrg, uuid.Nil); err != nil {
				return err
			}

			fmt.Printf("module %q enabled for org %s\n", moduleID, orgID)
			return nil
		},
	}

	cmd.Flags().StringVar(&orgID, "org", "", "organization ID (required)")
	cmd.MarkFlagRequired("org")
	return cmd
}

func (k *Kernel) moduleDisableCommand() *cobra.Command {
	var orgID string

	cmd := &cobra.Command{
		Use:   "disable [module-id]",
		Short: "Disable a module for an organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			parsedOrg, err := uuid.Parse(orgID)
			if err != nil {
				return fmt.Errorf("invalid org ID: %w", err)
			}

			moduleID := args[0]
			if err := k.DeactivateModule(context.Background(), moduleID, parsedOrg); err != nil {
				return err
			}

			fmt.Printf("module %q disabled for org %s\n", moduleID, orgID)
			return nil
		},
	}

	cmd.Flags().StringVar(&orgID, "org", "", "organization ID (required)")
	cmd.MarkFlagRequired("org")
	return cmd
}

func (k *Kernel) moduleStatusCommand() *cobra.Command {
	var orgID string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show module activation status for an organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			parsedOrg, err := uuid.Parse(orgID)
			if err != nil {
				return fmt.Errorf("invalid org ID: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "MODULE\tTYPE\tACTIVE")
			fmt.Fprintln(w, "──────\t────\t──────")
			for _, m := range k.Modules() {
				manifest := m.Manifest()
				active := k.IsModuleActive(manifest.ID, parsedOrg.String())
				fmt.Fprintf(w, "%s\t%s\t%v\n",
					manifest.ID, manifest.Type.String(), active)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&orgID, "org", "", "organization ID (required)")
	cmd.MarkFlagRequired("org")
	return cmd
}

func (k *Kernel) moduleDepsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deps",
		Short: "Show the module dependency graph",
		Run: func(cmd *cobra.Command, args []string) {
			manifests := k.Manifests()

			// Sort by ID for stable output.
			ids := make([]string, 0, len(manifests))
			for id := range manifests {
				ids = append(ids, id)
			}
			sort.Strings(ids)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "MODULE\tDEPENDS ON")
			fmt.Fprintln(w, "──────\t──────────")
			for _, id := range ids {
				m := manifests[id]
				deps := "(none)"
				if len(m.DependsOn) > 0 {
					deps = strings.Join(m.DependsOn, ", ")
				}
				fmt.Fprintf(w, "%s\t%s\n", id, deps)
			}
			w.Flush()
		},
	}
}

// ── org ──────────────────────────────────────────────────────────────────────

func (k *Kernel) orgCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Manage organizations",
	}

	cmd.AddCommand(
		k.orgProvisionCommand(),
		k.orgDeprovisionCommand(),
		k.orgListCommand(),
	)

	return cmd
}

func (k *Kernel) orgProvisionCommand() *cobra.Command {
	var adminEmail string

	cmd := &cobra.Command{
		Use:   "provision [org-id]",
		Short: "Provision a new organization",
		Long:  "Creates module activations for all core modules. The org record itself should already exist in the database.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			orgID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid org ID: %w", err)
			}

			if err := k.ProvisionOrg(context.Background(), orgID, uuid.Nil); err != nil {
				return err
			}

			fmt.Printf("org %s provisioned (%d core modules activated)\n", orgID, k.coreModuleCount())
			if adminEmail != "" {
				fmt.Printf("admin email: %s (invite should be sent by the IAM module)\n", adminEmail)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&adminEmail, "admin-email", "", "admin email for the new org")
	return cmd
}

func (k *Kernel) orgDeprovisionCommand() *cobra.Command {
	var confirm string

	cmd := &cobra.Command{
		Use:   "deprovision [org-id]",
		Short: "Deprovision an organization (deactivate all modules)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if confirm == "" {
				return fmt.Errorf("--confirm is required for safety")
			}

			if err := k.Boot(); err != nil {
				return err
			}

			orgID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid org ID: %w", err)
			}

			if err := k.DeprovisionOrg(context.Background(), orgID); err != nil {
				return err
			}

			fmt.Printf("org %s deprovisioned — all modules deactivated\n", orgID)
			return nil
		},
	}

	cmd.Flags().StringVar(&confirm, "confirm", "", "confirmation string (required)")
	cmd.MarkFlagRequired("confirm")
	return cmd
}

func (k *Kernel) orgListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List organizations with active modules",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			// Query distinct org_ids from module_activations.
			var activations []ModuleActivation
			if err := k.db.Select("DISTINCT org_id").Where("active = ?", true).Find(&activations).Error; err != nil {
				return fmt.Errorf("query orgs: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ORG ID\tACTIVE MODULES")
			fmt.Fprintln(w, "──────\t──────────────")

			// Group by org.
			orgModules := make(map[string]int)
			var allActivations []ModuleActivation
			k.db.Where("active = ?", true).Find(&allActivations)
			for _, a := range allActivations {
				orgModules[a.OrgID]++
			}

			// Sort org IDs for stable output.
			orgIDs := make([]string, 0, len(orgModules))
			for id := range orgModules {
				orgIDs = append(orgIDs, id)
			}
			sort.Strings(orgIDs)

			for _, id := range orgIDs {
				fmt.Fprintf(w, "%s\t%d\n", id, orgModules[id])
			}
			return w.Flush()
		},
	}
}
