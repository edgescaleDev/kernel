package kernel

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"go.edgescale.dev/kernel/sdk"
)

// cronCommand builds the `kernel cron` sub-command tree:
//
//	kernel cron start    — start the cron runner (long-running)
//	kernel cron list     — list all registered crons
//	kernel cron healthz  — liveness probe (exit 0/1)
//	kernel cron readyz   — readiness probe (exit 0/1)
func (k *Kernel) cronCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cron",
		Short: "Manage the cron job scheduler",
	}

	cmd.AddCommand(
		k.cronStartCommand(),
		k.cronListCommand(),
		k.cronHealthzCommand(),
		k.cronReadyzCommand(),
	)

	return cmd
}

// cronStartCommand boots the kernel in cron mode and starts the scheduler.
// Unlike `serve`, this does NOT start an HTTP server or register event subscribers.
// Hooks and crons are registered; events are publish-only.
func (k *Kernel) cronStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the cron scheduler (long-running process)",
		Long: `Boots the kernel in cron mode: initializes modules, registers hooks
and cron handlers. Does NOT start the HTTP server or event subscribers.

The scheduler runs until SIGTERM/SIGINT is received, then performs
a graceful shutdown waiting for in-progress jobs to finish.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. Boot infrastructure + validate deps.
			if err := k.Boot(); err != nil {
				return err
			}

			// 2. Init modules in cron mode (Init + Hooks + Crons, no Events/Routes).
			if err := k.initCronMode(); err != nil {
				return err
			}

			// 3. Sync module registry to database.
			if err := k.syncRegistry(); err != nil {
				return err
			}

			// 4. Build and start the cron runner.
			if err := k.startCronRunner(); err != nil {
				return err
			}

			k.logger.Info("cron scheduler running — send SIGTERM to stop")

			// 5. Block until shutdown signal.
			return k.WaitForSignal()
		},
	}
}

// cronListCommand lists all registered crons from module manifests.
// Does NOT require Boot() — manifests are available at compile time.
func (k *Kernel) cronListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered cron jobs",
		Run: func(cmd *cobra.Command, args []string) {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "MODULE\tCRON ID\tSCHEDULE\tTIMEZONE\tTIMEOUT\tDESCRIPTION")
			fmt.Fprintln(w, "──────\t───────\t────────\t────────\t───────\t───────────")

			for _, m := range k.orderedModules() {
				manifest := m.Manifest()
				for _, cron := range manifest.Crons {
					tz := cron.Timezone
					if tz == "" {
						tz = "UTC"
					}
					timeout := cron.Timeout
					if timeout == 0 {
						timeout = 5 * time.Minute
					}
					desc := ""
					if cron.Description != nil {
						if en, ok := cron.Description["en"]; ok {
							desc = en
						}
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
						manifest.ID, cron.ID, cron.Schedule, tz, timeout, desc)
				}
			}
			w.Flush()
		},
	}
}

// cronHealthzCommand checks the cron runner's liveness via Redis heartbeat.
// Exit 0 = alive, Exit 1 = no heartbeat found.
func (k *Kernel) cronHealthzCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "healthz",
		Short: "Check cron runner liveness (exit 0 = alive)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			if k.redis == nil {
				return fmt.Errorf("redis not configured — cannot check heartbeat")
			}

			ctx := context.Background()

			// Incrementally scan for cron heartbeat keys without blocking Redis.
			var (
				cursor   uint64
				foundAny bool
			)
			for {
				keys, nextCursor, err := k.redis.Scan(ctx, cursor, "cron:heartbeat:*", 100).Result()
				if err != nil {
					fmt.Println("UNHEALTHY: cannot reach Redis")
					os.Exit(1)
				}

				if len(keys) > 0 {
					foundAny = true
				}

				for _, key := range keys {
					val, err := k.redis.Get(ctx, key).Result()
					if err != nil {
						continue
					}
					ts, err := strconv.ParseInt(val, 10, 64)
					if err != nil {
						continue
					}
					age := time.Since(time.Unix(ts, 0))
					if age < 30*time.Second {
						fmt.Printf("HEALTHY: heartbeat from %s (%s ago)\n",
							key[len("cron:heartbeat:"):], age.Round(time.Second))
						return nil
					}
				}

				if nextCursor == 0 {
					break
				}
				cursor = nextCursor
			}

			if !foundAny {
				fmt.Println("UNHEALTHY: no cron runner heartbeat found")
				os.Exit(1)
			}

			fmt.Println("UNHEALTHY: all heartbeats stale")
			os.Exit(1)
			return nil
		},
	}
}

// cronReadyzCommand checks the cron runner's readiness:
//   - Redis heartbeat exists and is recent
//   - Database is reachable
func (k *Kernel) cronReadyzCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "readyz",
		Short: "Check cron runner readiness (exit 0 = ready)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k.Boot(); err != nil {
				return err
			}

			// Check database.
			if k.db != nil {
				sqlDB, err := k.db.DB()
				if err != nil || sqlDB.Ping() != nil {
					fmt.Println("NOT READY: database unreachable")
					os.Exit(1)
				}
			}

			// Check Redis heartbeat.
			if k.redis != nil {
				ctx := context.Background()
				var (
					cursor    uint64
					anyRecent bool
					foundAny  bool
				)
				for {
					keys, nextCursor, err := k.redis.Scan(ctx, cursor, "cron:heartbeat:*", 100).Result()
					if err != nil {
						fmt.Println("NOT READY: cannot reach Redis")
						os.Exit(1)
					}
					if len(keys) > 0 {
						foundAny = true
					}
					for _, key := range keys {
						val, _ := k.redis.Get(ctx, key).Result()
						ts, _ := strconv.ParseInt(val, 10, 64)
						if time.Since(time.Unix(ts, 0)) < 30*time.Second {
							anyRecent = true
						}
					}
					if nextCursor == 0 {
						break
					}
					cursor = nextCursor
				}
				if !foundAny {
					fmt.Println("NOT READY: no cron runner heartbeat found")
					os.Exit(1)
				}
				if !anyRecent {
					fmt.Println("NOT READY: all heartbeats stale")
					os.Exit(1)
				}
			}

			fmt.Println("READY")
			return nil
		},
	}
}

// ── kernel cron lifecycle ───────────────────────────────────────────────────

// initCronMode initializes modules for cron execution.
// Similar to the serve path but:
//   - Calls Init() on all modules (dependency order)
//   - Calls RegisterHooks() on HookModule implementations
//   - Calls RegisterCrons() on CronModule implementations
//   - Skips RegisterEvents() — publish-only mode (bus wired, no subscribers)
//   - Skips RouteHandlers() — no HTTP server
func (k *Kernel) initCronMode() error {
	k.logger.Info("initializing modules in cron mode")

	for _, id := range k.depOrder {
		m := k.moduleByID(id)
		if m == nil {
			continue
		}

		manifest := m.Manifest()

		// Build per-module context.
		modCtx := k.buildContext(manifest)

		// Init.
		k.logger.Info("initializing module", "id", id, "mode", "cron")
		if err := m.Init(modCtx); err != nil {
			return fmt.Errorf("init module %q: %w", id, err)
		}

		// RegisterHooks — hooks are domain-layer interceptors, needed in cron mode.
		if hm, ok := m.(sdk.HookModule); ok {
			k.logger.Info("registering hooks", "module", id)
			hm.RegisterHooks(k.hooks)
		}

		// RegisterCrons — collect handlers, validate against manifest.
		if cm, ok := m.(sdk.CronModule); ok {
			registry := sdk.NewCronRegistry()
			cm.RegisterCrons(registry)

			handlers := registry.Handlers()
			k.validateCronRegistrations(manifest, handlers)

			// Collect entries.
			for _, def := range manifest.Crons {
				handler, _ := handlers[def.ID]
				k.cronEntries = append(k.cronEntries, cronEntry{
					def:         def,
					moduleID:    manifest.ID,
					qualifiedID: manifest.ID + "." + def.ID,
					handler:     handler,
				})
			}
		}
	}

	k.logger.Info("cron mode init complete",
		"modules", len(k.depOrder),
		"crons", len(k.cronEntries),
	)
	return nil
}

// validateCronRegistrations checks that every Manifest.Cron has a handler
// and every registered handler has a Manifest.Cron. Panics on mismatch.
func (k *Kernel) validateCronRegistrations(manifest sdk.Manifest, handlers map[string]sdk.CronHandler) {
	// Build set of declared cron IDs.
	declared := make(map[string]bool, len(manifest.Crons))
	for _, c := range manifest.Crons {
		declared[c.ID] = true
	}

	// Check: every declared cron has a handler.
	for _, c := range manifest.Crons {
		if _, ok := handlers[c.ID]; !ok {
			panic(fmt.Sprintf(
				"kernel: module %q declares cron %q in Manifest but did not register a handler in RegisterCrons",
				manifest.ID, c.ID,
			))
		}
	}

	// Check: every registered handler has a manifest entry.
	for id := range handlers {
		if !declared[id] {
			panic(fmt.Sprintf(
				"kernel: module %q registered handler for cron %q but did not declare it in Manifest.Crons",
				manifest.ID, id,
			))
		}
	}
}

// startCronRunner builds the cronRunner from collected entries and starts it.
func (k *Kernel) startCronRunner() error {
	if len(k.cronEntries) == 0 {
		k.logger.Info("no cron jobs registered — scheduler will idle")
	}

	runner, err := newCronRunner(k.db, k.redis, k.lockProvider, k.logger.With("component", "cron"))
	if err != nil {
		return fmt.Errorf("create cron runner: %w", err)
	}

	for _, entry := range k.cronEntries {
		runner.register(entry)
	}

	if err := runner.start(context.Background()); err != nil {
		return fmt.Errorf("start cron runner: %w", err)
	}

	k.cronRunner = runner
	return nil
}

// moduleByID returns the module with the given manifest ID, or nil.
func (k *Kernel) moduleByID(id string) sdk.Module {
	for _, m := range k.modules {
		if m.Manifest().ID == id {
			return m
		}
	}
	return nil
}
