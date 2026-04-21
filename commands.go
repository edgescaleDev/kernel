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

	"go.edgescale.dev/kernel/internal"
	"go.edgescale.dev/kernel/sdk"
)

// buildRootCommand constructs the Cobra command tree for the kernel.
// Called by Execute() — consumers don't interact with this directly.
func (k *Kernel) buildRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "kernel",
		Short: "Kernel CLI — manage modules, migrations, and tenants",
		// Silence Cobra's default usage/error printing — we handle it via slog.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		k.serveCommand(),
		k.migrateCommand(),
		k.moduleCommand(),
		k.tenantCommand(),
		k.platformCommand(),
		k.cronCommand(),
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
	cmd.AddCommand(k.migrateRollbackCommand())
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

			var applied []internal.SchemaMigration
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

func (k *Kernel) migrateRollbackCommand() *cobra.Command {
	var moduleID string
	var steps int

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback the last N migrations for a module",
		Long: `Reverts applied migrations by executing the corresponding .down.sql files.
Each migration is rolled back in its own transaction. If a .down.sql file
is missing, the rollback stops with an error.

Examples:
  kernel migrate rollback --module billing
  kernel migrate rollback --module billing --steps 3
  kernel migrate rollback --module kernel --steps 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			if err := k.Rollback(moduleID, steps); err != nil {
				return err
			}

			fmt.Printf("rollback completed for module %q\n", moduleID)
			return nil
		},
	}

	cmd.Flags().StringVar(&moduleID, "module", "", "module ID to rollback (required)")
	cmd.Flags().IntVar(&steps, "steps", 1, "number of migrations to rollback")
	cmd.MarkFlagRequired("module")
	return cmd
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
			for _, m := range k.orderedModules() {
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
	var tenantID string

	cmd := &cobra.Command{
		Use:   "enable [module-id]",
		Short: "Enable a module for a tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			parsedTenant, err := uuid.Parse(tenantID)
			if err != nil {
				return fmt.Errorf("invalid tenant ID: %w", err)
			}

			moduleID := args[0]
			if _, exists := k.manifests[moduleID]; !exists {
				return fmt.Errorf("module %q not registered", moduleID)
			}

			// Use a zero UUID for activated_by in CLI context.
			if err := k.ActivateModule(context.Background(), moduleID, parsedTenant, uuid.Nil); err != nil {
				return err
			}

			fmt.Printf("module %q enabled for tenant %s\n", moduleID, tenantID)
			return nil
		},
	}

	cmd.Flags().StringVar(&tenantID, "tenant", "", "tenant ID (required)")
	cmd.MarkFlagRequired("tenant")
	return cmd
}

func (k *Kernel) moduleDisableCommand() *cobra.Command {
	var tenantID string

	cmd := &cobra.Command{
		Use:   "disable [module-id]",
		Short: "Disable a module for a tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			parsedTenant, err := uuid.Parse(tenantID)
			if err != nil {
				return fmt.Errorf("invalid tenant ID: %w", err)
			}

			moduleID := args[0]
			if err := k.DeactivateModule(context.Background(), moduleID, parsedTenant); err != nil {
				return err
			}

			fmt.Printf("module %q disabled for tenant %s\n", moduleID, tenantID)
			return nil
		},
	}

	cmd.Flags().StringVar(&tenantID, "tenant", "", "tenant ID (required)")
	cmd.MarkFlagRequired("tenant")
	return cmd
}

func (k *Kernel) moduleStatusCommand() *cobra.Command {
	var tenantID string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show module activation status for a tenant",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			parsedTenant, err := uuid.Parse(tenantID)
			if err != nil {
				return fmt.Errorf("invalid tenant ID: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "MODULE\tTYPE\tACTIVE")
			fmt.Fprintln(w, "──────\t────\t──────")
			for _, m := range k.orderedModules() {
				manifest := m.Manifest()
				active := k.isModuleActive(manifest.ID, parsedTenant.String())
				fmt.Fprintf(w, "%s\t%s\t%v\n",
					manifest.ID, manifest.Type.String(), active)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&tenantID, "tenant", "", "tenant ID (required)")
	cmd.MarkFlagRequired("tenant")
	return cmd
}

func (k *Kernel) moduleDepsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deps",
		Short: "Show the module dependency graph",
		Run: func(cmd *cobra.Command, args []string) {
			manifests := k.allManifests()

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

// ── tenant ──────────────────────────────────────────────────────────────────

func (k *Kernel) tenantCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "Manage tenants",
	}

	cmd.AddCommand(
		k.tenantProvisionCommand(),
		k.tenantDeprovisionCommand(),
		k.tenantListCommand(),
	)

	return cmd
}

func (k *Kernel) tenantProvisionCommand() *cobra.Command {
	var adminEmail string

	cmd := &cobra.Command{
		Use:   "provision [tenant-id]",
		Short: "Provision a new tenant",
		Long:  "Creates module activations for all core modules. The tenant record itself should already exist in the database.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			tenantID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid tenant ID: %w", err)
			}

			if err := k.ProvisionTenant(context.Background(), tenantID, uuid.Nil); err != nil {
				return err
			}

			fmt.Printf("tenant %s provisioned (%d core modules activated)\n", tenantID, k.coreModuleCount())
			if adminEmail != "" {
				fmt.Printf("admin email: %s (invite should be sent by the IAM module)\n", adminEmail)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&adminEmail, "admin-email", "", "admin email for the new tenant")
	return cmd
}

func (k *Kernel) tenantDeprovisionCommand() *cobra.Command {
	var confirm string

	cmd := &cobra.Command{
		Use:   "deprovision [tenant-id]",
		Short: "Deprovision a tenant (deactivate all modules)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if confirm == "" {
				return fmt.Errorf("--confirm is required for safety")
			}

			if err := k.Boot(); err != nil {
				return err
			}

			tenantID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid tenant ID: %w", err)
			}

			if err := k.DeprovisionTenant(context.Background(), tenantID); err != nil {
				return err
			}

			fmt.Printf("tenant %s deprovisioned — all modules deactivated\n", tenantID)
			return nil
		},
	}

	cmd.Flags().StringVar(&confirm, "confirm", "", "confirmation string (required)")
	cmd.MarkFlagRequired("confirm")
	return cmd
}

func (k *Kernel) tenantListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tenants with active modules",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			// Query distinct tenant_ids from module_activations.
			var activations []internal.ModuleActivation
			if err := k.db.Select("DISTINCT tenant_id").Where("active = ?", true).Find(&activations).Error; err != nil {
				return fmt.Errorf("query tenants: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "TENANT ID\tACTIVE MODULES")
			fmt.Fprintln(w, "─────────\t──────────────")

			// Group by tenant.
			tenantModules := make(map[string]int)
			var allActivations []internal.ModuleActivation
			k.db.Where("active = ?", true).Find(&allActivations)
			for _, a := range allActivations {
				tenantModules[a.TenantID]++
			}

			// Sort tenant IDs for stable output.
			tenantIDs := make([]string, 0, len(tenantModules))
			for id := range tenantModules {
				tenantIDs = append(tenantIDs, id)
			}
			sort.Strings(tenantIDs)

			for _, id := range tenantIDs {
				fmt.Fprintf(w, "%s\t%d\n", id, tenantModules[id])
			}
			return w.Flush()
		},
	}
}

// ── platform ─────────────────────────────────────────────────────────────────

func (k *Kernel) platformCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "platform",
		Short: "Manage platform administrators",
	}

	cmd.AddCommand(
		k.platformGrantCommand(),
		k.platformRevokeCommand(),
		k.platformListCommand(),
	)

	return cmd
}

func (k *Kernel) platformGrantCommand() *cobra.Command {
	var roleName string

	cmd := &cobra.Command{
		Use:   "grant [user-id]",
		Short: "Grant a platform role to a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			if k.adminResolver == nil {
				return fmt.Errorf("platform admin not configured — register an IAM module with admin support")
			}

			userID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid user ID: %w", err)
			}

			// Delegate to the AdminResolver if it supports role management.
			if mgr, ok := k.adminResolver.(sdk.PlatformManager); ok {
				if err := mgr.GrantRole(context.Background(), userID, roleName); err != nil {
					return err
				}
				fmt.Printf("granted platform role %q to user %s\n", roleName, userID)
				return nil
			}

			return fmt.Errorf("admin resolver does not support role management")
		},
	}

	cmd.Flags().StringVar(&roleName, "role", "platform_admin", "platform role slug")
	return cmd
}

func (k *Kernel) platformRevokeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke [user-id]",
		Short: "Revoke all platform roles from a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			if k.adminResolver == nil {
				return fmt.Errorf("platform admin not configured — register an IAM module with admin support")
			}

			userID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid user ID: %w", err)
			}

			if mgr, ok := k.adminResolver.(sdk.PlatformManager); ok {
				if err := mgr.RevokeAllRoles(context.Background(), userID); err != nil {
					return err
				}
				fmt.Printf("revoked all platform roles from user %s\n", userID)
				return nil
			}

			return fmt.Errorf("admin resolver does not support role management")
		},
	}

	return cmd
}

func (k *Kernel) platformListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all platform administrators",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			if k.adminResolver == nil {
				return fmt.Errorf("platform admin not configured — register an IAM module with admin support")
			}

			if mgr, ok := k.adminResolver.(sdk.PlatformManager); ok {
				admins, err := mgr.ListAdmins(context.Background())
				if err != nil {
					return err
				}

				w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "USER ID\tNAME\tEMAIL\tROLE")
				fmt.Fprintln(w, "───────\t────\t─────\t────")
				for _, a := range admins {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.UserID, a.Name, a.Email, a.Role)
				}
				return w.Flush()
			}

			return fmt.Errorf("admin resolver does not support listing admins")
		},
	}
}
