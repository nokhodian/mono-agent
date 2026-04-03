package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"

	cfgpkg "github.com/nokhodian/mono-agent/internal/config"
	"github.com/nokhodian/mono-agent/internal/connections"
	"github.com/nokhodian/mono-agent/internal/nodes"
	"github.com/nokhodian/mono-agent/internal/scheduler"
	"github.com/nokhodian/mono-agent/internal/storage"
	"github.com/nokhodian/mono-agent/internal/workflow"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// newHybridStore creates a HybridWorkflowStore that reads from both
// the JSON file store (~/.monoes/workflows/) and the SQLite database.
// This ensures workflows created by both the CLI and the Wails GUI are visible.
func newHybridStore(db *storage.Database) *workflow.HybridWorkflowStore {
	sqlStore := workflow.NewSQLiteWorkflowStore(db.DB)
	fileStore, err := workflow.NewWorkflowFileStore(expandPath("~/.monoes/workflows"))
	if err != nil {
		// If file store can't be created, wrap SQLite-only in a hybrid shell
		// so callers always get the same type.
		return workflow.NewHybridWorkflowStore(nil, sqlStore)
	}
	return workflow.NewHybridWorkflowStore(fileStore, sqlStore)
}

// buildEngine constructs a fully wired WorkflowEngine suitable for CLI use.
// It creates its own scheduler (no action executor or store needed for workflow triggers).
func buildEngine(cfg *globalConfig) (*workflow.WorkflowEngine, error) {
	db, err := initDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	registry := buildNodeRegistry(cfg.Verbose, db.DB)

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	if !cfg.Verbose {
		logger = logger.Level(zerolog.WarnLevel)
	}

	// Set up browser session provider, bot registry, and config manager
	// so browser/social nodes work in workflows.
	sp := &cliSessionProvider{db: db.DB}
	nodes.SetGlobalSessionProvider(sp)
	nodes.SetGlobalBotRegistry(&cliBotRegistry{})
	nodes.SetGlobalCredentialStore(connections.NewStore(db.DB))

	cfgLogger := zerolog.New(os.Stderr).Level(zerolog.WarnLevel)
	var cfgStore cfgpkg.ConfigStore
	if cfgDB, err2 := initDB(cfg); err2 == nil {
		cfgStore = &cfgpkg.DBConfigStore{DB: cfgDB}
	}
	apiClient := cfgpkg.NewAPIClient(cfgLogger)
	rawCfgMgr := cfgpkg.NewConfigManager(expandPath("~/.monoes/configs"), cfgStore, apiClient, cfgLogger)
	nodes.SetGlobalConfigMgr(&cfgpkg.ConfigManagerAdapter{Mgr: rawCfgMgr})

	sched := scheduler.NewScheduler(nil, nil, logger)
	sched.Start()

	engCfg := workflow.EngineConfig{
		MaxConcurrent:  5,
		QueueCapacity:  1000,
		PruneInterval:  time.Hour,
		MaxExecHistory: 500,
	}

	hybridStore := newHybridStore(db)
	engine := workflow.NewWorkflowEngineWithStore(hybridStore, db.DB, sched, registry, engCfg, logger)
	return engine, nil
}

// newWorkflowCmd returns the parent `workflow` cobra command with all subcommands attached.
func newWorkflowCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage and run workflows",
		Long:  "Create, list, run, activate, deactivate, delete, and inspect executions of workflows.",
	}

	cmd.AddCommand(
		newWorkflowListCmd(cfg),
		newWorkflowGetCmd(cfg),
		newWorkflowCreateCmd(cfg),
		newWorkflowImportCmd(cfg),
		newWorkflowExportCmd(cfg),
		newWorkflowRunCmd(cfg),
		newWorkflowActivateCmd(cfg),
		newWorkflowDeactivateCmd(cfg),
		newWorkflowDeleteCmd(cfg),
		newWorkflowExecutionsCmd(cfg),
		newWorkflowNodeCmd(cfg),
		newWorkflowConnectCmd(cfg),
		newWorkflowDisconnectCmd(cfg),
		newWorkflowMigrateCmd(cfg),
	)

	return cmd
}

// newWorkflowListCmd lists all workflows.
func newWorkflowListCmd(cfg *globalConfig) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			workflows, err := store.ListWorkflows(ctx)
			if err != nil {
				return fmt.Errorf("list workflows: %w", err)
			}

			if jsonOut || cfg.JSONOutput {
				return json.NewEncoder(os.Stdout).Encode(workflows)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tACTIVE\tVERSION\tUPDATED AT")
			for _, wf := range workflows {
				active := "false"
				if wf.IsActive {
					active = "true"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
					wf.ID, wf.Name, active, wf.Version,
					wf.UpdatedAt.Format(time.RFC3339),
				)
			}
			return w.Flush()
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output in JSON format")
	return cmd
}

// newWorkflowGetCmd prints a single workflow as JSON.
func newWorkflowGetCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Print a workflow as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			wf, err := store.GetWorkflow(ctx, args[0])
			if err != nil {
				return fmt.Errorf("get workflow: %w", err)
			}
			if wf == nil {
				return fmt.Errorf("workflow %q not found", args[0])
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(wf)
		},
	}
}

// newWorkflowRunCmd manually triggers a workflow and polls for completion.
func newWorkflowRunCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "run <id>",
		Short: "Manually trigger a workflow and wait for it to complete",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := args[0]

			engine, err := buildEngine(cfg)
			if err != nil {
				return fmt.Errorf("build engine: %w", err)
			}

			ctx := context.Background()
			if err := engine.Start(ctx); err != nil {
				return fmt.Errorf("start engine: %w", err)
			}
			defer engine.Stop() //nolint:errcheck

			executionID, err := engine.TriggerWorkflow(ctx, workflowID, map[string]interface{}{})
			if err != nil {
				return fmt.Errorf("trigger workflow: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Execution started: %s\n", executionID)

			// Poll until the execution leaves RUNNING/QUEUED or times out.
			deadline := time.Now().Add(5 * time.Minute)
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					exec, err := engine.GetExecution(ctx, executionID)
					if err != nil {
						return fmt.Errorf("poll execution: %w", err)
					}
					switch exec.Status {
					case "RUNNING", "QUEUED":
						if time.Now().After(deadline) {
							return fmt.Errorf("timed out waiting for execution %s (still %s)", executionID, exec.Status)
						}
						// keep polling
					default:
						errMsg := exec.ErrorMessage
						if errMsg != "" {
							fmt.Fprintf(os.Stdout, "Status: %s\nError:  %s\n", exec.Status, errMsg)
						} else {
							fmt.Fprintf(os.Stdout, "Status: %s\n", exec.Status)
						}
						return nil
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		},
	}
}

// newWorkflowActivateCmd enables a workflow and registers its triggers.
func newWorkflowActivateCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "activate <id>",
		Short: "Activate a workflow and start its triggers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := buildEngine(cfg)
			if err != nil {
				return fmt.Errorf("build engine: %w", err)
			}

			ctx := context.Background()
			if err := engine.Start(ctx); err != nil {
				return fmt.Errorf("start engine: %w", err)
			}
			defer engine.Stop() //nolint:errcheck

			if err := engine.ActivateWorkflow(ctx, args[0]); err != nil {
				return fmt.Errorf("activate workflow: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Workflow %s activated.\n", args[0])
			return nil
		},
	}
}

// newWorkflowDeactivateCmd disables a workflow and unregisters its triggers.
func newWorkflowDeactivateCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate <id>",
		Short: "Deactivate a workflow and stop its triggers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := buildEngine(cfg)
			if err != nil {
				return fmt.Errorf("build engine: %w", err)
			}

			ctx := context.Background()
			if err := engine.Start(ctx); err != nil {
				return fmt.Errorf("start engine: %w", err)
			}
			defer engine.Stop() //nolint:errcheck

			if err := engine.DeactivateWorkflow(ctx, args[0]); err != nil {
				return fmt.Errorf("deactivate workflow: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Workflow %s deactivated.\n", args[0])
			return nil
		},
	}
}

// newWorkflowDeleteCmd deletes a workflow (with confirmation unless --force).
func newWorkflowDeleteCmd(cfg *globalConfig) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := args[0]

			if !force {
				fmt.Fprintf(os.Stdout, "Delete workflow %q? This is irreversible. [y/N] ", workflowID)
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(strings.ToLower(answer))
				if answer != "y" && answer != "yes" {
					fmt.Fprintln(os.Stdout, "Aborted.")
					return nil
				}
			}

			engine, err := buildEngine(cfg)
			if err != nil {
				return fmt.Errorf("build engine: %w", err)
			}

			ctx := context.Background()
			if err := engine.Start(ctx); err != nil {
				return fmt.Errorf("start engine: %w", err)
			}
			defer engine.Stop() //nolint:errcheck

			if err := engine.DeleteWorkflow(ctx, workflowID); err != nil {
				return fmt.Errorf("delete workflow: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Workflow %s deleted.\n", workflowID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	return cmd
}

// newWorkflowExecutionsCmd lists recent executions for a workflow.
func newWorkflowExecutionsCmd(cfg *globalConfig) *cobra.Command {
	var limit int
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "executions <workflow-id>",
		Short: "List recent executions for a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			executions, err := store.ListExecutions(ctx, args[0], limit)
			if err != nil {
				return fmt.Errorf("list executions: %w", err)
			}

			if jsonOut || cfg.JSONOutput {
				return json.NewEncoder(os.Stdout).Encode(executions)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATUS\tTRIGGER TYPE\tSTARTED AT\tFINISHED AT\tERROR")
			for _, e := range executions {
				startedAt := ""
				if e.StartedAt != nil {
					startedAt = e.StartedAt.Format(time.RFC3339)
				}
				finishedAt := ""
				if e.FinishedAt != nil {
					finishedAt = e.FinishedAt.Format(time.RFC3339)
				}
				errStr := e.ErrorMessage
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					e.ID, e.Status, e.TriggerType,
					startedAt, finishedAt, errStr,
				)
			}
			return w.Flush()
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of executions to show (0 = all)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output in JSON format")
	return cmd
}

// newWorkflowCreateCmd creates a blank workflow and prints its ID.
func newWorkflowCreateCmd(cfg *globalConfig) *cobra.Command {
	var description string
	var active bool

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new blank workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			now := time.Now().UTC()
			wf := &workflow.Workflow{
				ID:          uuid.New().String(),
				Name:        args[0],
				Description: description,
				IsActive:    active,
				Version:     1,
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			store := newHybridStore(db)
			ctx := context.Background()
			if err := store.CreateWorkflow(ctx, wf); err != nil {
				return fmt.Errorf("create workflow: %w", err)
			}

			if cfg.JSONOutput {
				return json.NewEncoder(os.Stdout).Encode(wf)
			}
			fmt.Fprintf(os.Stdout, "Created workflow: %s  (id: %s)\n", wf.Name, wf.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Workflow description")
	cmd.Flags().BoolVar(&active, "active", false, "Mark workflow as active immediately")
	return cmd
}

// newWorkflowImportCmd imports a full workflow definition from a JSON file.
func newWorkflowImportCmd(cfg *globalConfig) *cobra.Command {
	var inputFile string
	var overwrite bool

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a workflow from a JSON file (--file or stdin)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw []byte
			var err error
			if inputFile != "" {
				raw, err = os.ReadFile(inputFile)
			} else {
				raw, err = io.ReadAll(os.Stdin)
			}
			if err != nil {
				return fmt.Errorf("read input: %w", err)
			}

			// Try to parse as a WorkflowFile (new JSON file format with "type",
			// "source", "target" keys). Fall back to direct Workflow unmarshal for
			// legacy exports that already use "node_type", "source_node_id" etc.
			wf, parseErr := workflow.ParseWorkflowFileBytes(raw)
			if parseErr != nil {
				// Legacy format fallback.
				var legacyWF workflow.Workflow
				if err2 := json.Unmarshal(raw, &legacyWF); err2 != nil {
					return fmt.Errorf("parse JSON: %w", parseErr)
				}
				wf = legacyWF
			}

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			now := time.Now().UTC()

			// Assign a fresh ID unless --overwrite is requested and one is present.
			if !overwrite || wf.ID == "" {
				wf.ID = uuid.New().String()
			}
			wf.CreatedAt = now
			wf.UpdatedAt = now

			if err := store.CreateWorkflow(ctx, &wf); err != nil {
				return fmt.Errorf("save workflow: %w", err)
			}

			// Parse config for each node before saving.
			nodes := wf.Nodes
			for i := range nodes {
				nodes[i].WorkflowID = wf.ID
				if nodes[i].ID == "" {
					nodes[i].ID = uuid.New().String()
				}
				if nodes[i].Config != nil {
					if err := nodes[i].MarshalConfig(); err != nil {
						return fmt.Errorf("marshal node config: %w", err)
					}
				}
			}
			if err := store.SaveWorkflowNodes(ctx, wf.ID, nodes); err != nil {
				return fmt.Errorf("save nodes: %w", err)
			}

			conns := wf.Connections
			for i := range conns {
				conns[i].WorkflowID = wf.ID
				if conns[i].ID == "" {
					conns[i].ID = uuid.New().String()
				}
			}
			if err := store.SaveWorkflowConnections(ctx, wf.ID, conns); err != nil {
				return fmt.Errorf("save connections: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Imported workflow %q as id: %s  (%d nodes, %d connections)\n",
				wf.Name, wf.ID, len(nodes), len(conns))
			return nil
		},
	}

	cmd.Flags().StringVarP(&inputFile, "file", "f", "", "Path to JSON file (default: stdin)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Keep the id from the file instead of generating a new one")
	return cmd
}

// newWorkflowExportCmd exports a workflow as JSON.
func newWorkflowExportCmd(cfg *globalConfig) *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a workflow as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			wf, err := store.GetWorkflow(ctx, args[0])
			if err != nil {
				return fmt.Errorf("get workflow: %w", err)
			}
			if wf == nil {
				return fmt.Errorf("workflow %q not found", args[0])
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if outputFile != "" {
				f, err := os.Create(outputFile)
				if err != nil {
					return fmt.Errorf("create output file: %w", err)
				}
				defer f.Close()
				enc = json.NewEncoder(f)
				enc.SetIndent("", "  ")
				if err := enc.Encode(wf); err != nil {
					return err
				}
				fmt.Fprintf(os.Stdout, "Exported workflow %q to %s\n", wf.Name, outputFile)
				return nil
			}
			return enc.Encode(wf)
		},
	}

	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Write to file instead of stdout")
	return cmd
}

// ── node subcommand group ────────────────────────────────────────────────────

// newWorkflowNodeCmd is the `workflow node` parent command.
func newWorkflowNodeCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Manage nodes within a workflow",
	}
	cmd.AddCommand(
		newWorkflowNodeAddCmd(cfg),
		newWorkflowNodeListCmd(cfg),
		newWorkflowNodeSetCmd(cfg),
		newWorkflowNodeRemoveCmd(cfg),
	)
	return cmd
}

// newWorkflowNodeAddCmd adds a node to a workflow.
func newWorkflowNodeAddCmd(cfg *globalConfig) *cobra.Command {
	var nodeType, name, configJSON string
	var posX, posY float64

	cmd := &cobra.Command{
		Use:   "add <workflow-id>",
		Short: "Add a node to a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := args[0]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			// Ensure workflow exists.
			wf, err := store.GetWorkflow(ctx, workflowID)
			if err != nil || wf == nil {
				return fmt.Errorf("workflow %q not found", workflowID)
			}

			// Fetch existing nodes so we can append.
			existing := wf.Nodes

			// Parse --config JSON.
			var config map[string]interface{}
			if configJSON != "" {
				if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
					return fmt.Errorf("parse --config JSON: %w", err)
				}
			} else {
				config = make(map[string]interface{})
			}

			newNode := workflow.WorkflowNode{
				ID:         uuid.New().String(),
				WorkflowID: workflowID,
				Type:       nodeType,
				Name:       name,
				Config:     config,
				PositionX:  posX,
				PositionY:  posY,
			}
			if err := newNode.MarshalConfig(); err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}

			existing = append(existing, newNode)
			if err := store.SaveWorkflowNodes(ctx, workflowID, existing); err != nil {
				return fmt.Errorf("save nodes: %w", err)
			}

			if cfg.JSONOutput {
				return json.NewEncoder(os.Stdout).Encode(newNode)
			}
			fmt.Fprintf(os.Stdout, "Added node %s (type: %s, id: %s)\n", newNode.Name, newNode.Type, newNode.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&nodeType, "type", "", "Node type (e.g. core.if, trigger.schedule) [required]")
	cmd.Flags().StringVar(&name, "name", "", "Display name for the node [required]")
	cmd.Flags().StringVar(&configJSON, "config", "", "Node configuration as JSON object")
	cmd.Flags().Float64Var(&posX, "x", 0, "Canvas X position")
	cmd.Flags().Float64Var(&posY, "y", 0, "Canvas Y position")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// newWorkflowNodeListCmd lists the nodes in a workflow.
func newWorkflowNodeListCmd(cfg *globalConfig) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list <workflow-id>",
		Short: "List nodes in a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			wf, err := store.GetWorkflow(ctx, args[0])
			if err != nil || wf == nil {
				return fmt.Errorf("workflow %q not found", args[0])
			}

			if jsonOut || cfg.JSONOutput {
				return json.NewEncoder(os.Stdout).Encode(wf.Nodes)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tTYPE\tNAME\tX\tY\tCONFIG")
			for _, n := range wf.Nodes {
				configStr := n.ConfigRaw
				if len(configStr) > 60 {
					configStr = configStr[:57] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%.0f\t%.0f\t%s\n",
					n.ID, n.Type, n.Name, n.PositionX, n.PositionY, configStr)
			}
			return w.Flush()
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output in JSON format")
	return cmd
}

// newWorkflowNodeSetCmd updates a node's configuration and/or position.
func newWorkflowNodeSetCmd(cfg *globalConfig) *cobra.Command {
	var configJSON, name string
	var posX, posY float64
	var setPosX, setPosY bool

	cmd := &cobra.Command{
		Use:   "set <workflow-id> <node-id>",
		Short: "Update a node's config, name, or position",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID, nodeID := args[0], args[1]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			wf, err := store.GetWorkflow(ctx, workflowID)
			if err != nil || wf == nil {
				return fmt.Errorf("workflow %q not found", workflowID)
			}

			found := false
			for i := range wf.Nodes {
				if wf.Nodes[i].ID != nodeID {
					continue
				}
				found = true
				if name != "" {
					wf.Nodes[i].Name = name
				}
				if configJSON != "" {
					var config map[string]interface{}
					if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
						return fmt.Errorf("parse --config JSON: %w", err)
					}
					wf.Nodes[i].Config = config
					if err := wf.Nodes[i].MarshalConfig(); err != nil {
						return fmt.Errorf("marshal config: %w", err)
					}
				}
				if setPosX {
					wf.Nodes[i].PositionX = posX
				}
				if setPosY {
					wf.Nodes[i].PositionY = posY
				}
				break
			}
			if !found {
				return fmt.Errorf("node %q not found in workflow %q", nodeID, workflowID)
			}

			if err := store.SaveWorkflowNodes(ctx, workflowID, wf.Nodes); err != nil {
				return fmt.Errorf("save nodes: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Node %s updated.\n", nodeID)
			return nil
		},
	}

	cmd.Flags().StringVar(&configJSON, "config", "", "New configuration as JSON object")
	cmd.Flags().StringVar(&name, "name", "", "New display name")
	cmd.Flags().Float64Var(&posX, "x", 0, "Canvas X position")
	cmd.Flags().Float64Var(&posY, "y", 0, "Canvas Y position")
	// Detect whether --x/--y were actually provided.
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		setPosX = cmd.Flags().Changed("x")
		setPosY = cmd.Flags().Changed("y")
		return nil
	}
	return cmd
}

// newWorkflowNodeRemoveCmd removes a node (and its connections) from a workflow.
func newWorkflowNodeRemoveCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <workflow-id> <node-id>",
		Short: "Remove a node from a workflow",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID, nodeID := args[0], args[1]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			wf, err := store.GetWorkflow(ctx, workflowID)
			if err != nil || wf == nil {
				return fmt.Errorf("workflow %q not found", workflowID)
			}

			newNodes := wf.Nodes[:0]
			found := false
			for _, n := range wf.Nodes {
				if n.ID == nodeID {
					found = true
					continue
				}
				newNodes = append(newNodes, n)
			}
			if !found {
				return fmt.Errorf("node %q not found in workflow %q", nodeID, workflowID)
			}

			// Drop connections that reference the removed node.
			newConns := wf.Connections[:0]
			for _, c := range wf.Connections {
				if c.SourceNodeID == nodeID || c.TargetNodeID == nodeID {
					continue
				}
				newConns = append(newConns, c)
			}

			if err := store.SaveWorkflowNodes(ctx, workflowID, newNodes); err != nil {
				return fmt.Errorf("save nodes: %w", err)
			}
			if err := store.SaveWorkflowConnections(ctx, workflowID, newConns); err != nil {
				return fmt.Errorf("save connections: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Node %s removed.\n", nodeID)
			return nil
		},
	}
}

// ── connect / disconnect ─────────────────────────────────────────────────────

// newWorkflowConnectCmd adds an edge between two nodes.
func newWorkflowConnectCmd(cfg *globalConfig) *cobra.Command {
	var fromStr, toStr string

	cmd := &cobra.Command{
		Use:   "connect <workflow-id>",
		Short: "Connect two nodes  (--from nodeID:handle --to nodeID:handle)",
		Long: `Add a connection between two nodes.

  --from and --to accept "nodeID:handle" or just "nodeID" (handle defaults to "main").

  Example:
    monoes workflow connect wf1 --from abc123:main --to def456:main`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := args[0]

			srcID, srcHandle, err := parseNodeHandle(fromStr)
			if err != nil {
				return fmt.Errorf("--from: %w", err)
			}
			dstID, dstHandle, err := parseNodeHandle(toStr)
			if err != nil {
				return fmt.Errorf("--to: %w", err)
			}

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			wf, err := store.GetWorkflow(ctx, workflowID)
			if err != nil || wf == nil {
				return fmt.Errorf("workflow %q not found", workflowID)
			}

			conn := workflow.WorkflowConnection{
				ID:           uuid.New().String(),
				WorkflowID:   workflowID,
				SourceNodeID: srcID,
				SourceHandle: srcHandle,
				TargetNodeID: dstID,
				TargetHandle: dstHandle,
				Position:     len(wf.Connections),
			}
			wf.Connections = append(wf.Connections, conn)

			if err := store.SaveWorkflowConnections(ctx, workflowID, wf.Connections); err != nil {
				return fmt.Errorf("save connections: %w", err)
			}

			if cfg.JSONOutput {
				return json.NewEncoder(os.Stdout).Encode(conn)
			}
			fmt.Fprintf(os.Stdout, "Connected %s:%s → %s:%s  (id: %s)\n",
				srcID, srcHandle, dstID, dstHandle, conn.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromStr, "from", "", "Source node: nodeID[:handle]  (required)")
	cmd.Flags().StringVar(&toStr, "to", "", "Target node: nodeID[:handle]  (required)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// newWorkflowDisconnectCmd removes a connection by its ID.
func newWorkflowDisconnectCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "disconnect <workflow-id> <connection-id>",
		Short: "Remove a connection from a workflow",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID, connID := args[0], args[1]

			db, err := initDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			store := newHybridStore(db)
			ctx := context.Background()

			wf, err := store.GetWorkflow(ctx, workflowID)
			if err != nil || wf == nil {
				return fmt.Errorf("workflow %q not found", workflowID)
			}

			newConns := wf.Connections[:0]
			found := false
			for _, c := range wf.Connections {
				if c.ID == connID {
					found = true
					continue
				}
				newConns = append(newConns, c)
			}
			if !found {
				return fmt.Errorf("connection %q not found in workflow %q", connID, workflowID)
			}

			if err := store.SaveWorkflowConnections(ctx, workflowID, newConns); err != nil {
				return fmt.Errorf("save connections: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Connection %s removed.\n", connID)
			return nil
		},
	}
}

// newWorkflowMigrateCmd migrates workflows from SQLite to JSON files.
func newWorkflowMigrateCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Migrate workflows from SQLite to JSON files",
		RunE:  runWorkflowMigrate(cfg),
	}
}

func runWorkflowMigrate(cfg *globalConfig) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		db, err := initDB(cfg)
		if err != nil {
			return fmt.Errorf("open sqlite store: %w", err)
		}
		defer db.Close()

		sqliteStore := workflow.NewSQLiteWorkflowStore(db.DB)

		wfDir := filepath.Join(os.Getenv("HOME"), ".monoes", "workflows")
		fileStore, err := workflow.NewWorkflowFileStore(wfDir)
		if err != nil {
			return fmt.Errorf("open file store: %w", err)
		}

		workflows, err := sqliteStore.ListWorkflows(ctx)
		if err != nil {
			return fmt.Errorf("list workflows from sqlite: %w", err)
		}

		fmt.Printf("Found %d workflows in SQLite. Migrating to %s...\n", len(workflows), wfDir)

		var migrated, skipped int
		for _, wf := range workflows {
			full, err := sqliteStore.GetWorkflow(ctx, wf.ID)
			if err != nil {
				fmt.Printf("  SKIP %s (%s): load error: %v\n", wf.ID, wf.Name, err)
				skipped++
				continue
			}
			if full == nil {
				skipped++
				continue
			}

			for i, n := range full.Nodes {
				if n.Schema == nil {
					schema, err := workflow.LoadDefaultSchema(n.Type)
					if err != nil {
						schema = &workflow.NodeSchema{Fields: []workflow.NodeSchemaField{}}
					}
					full.Nodes[i].Schema = schema
				}
			}

			if err := fileStore.SaveWorkflow(ctx, full); err != nil {
				fmt.Printf("  SKIP %s (%s): write error: %v\n", wf.ID, wf.Name, err)
				skipped++
				continue
			}
			fmt.Printf("  OK   %s (%s)\n", full.ID, full.Name)
			migrated++
		}

		fmt.Printf("\nMigration complete: %d migrated, %d skipped.\n", migrated, skipped)
		fmt.Println("SQLite data was NOT modified — safe to roll back.")
		return nil
	}
}

// parseNodeHandle parses "nodeID:handle" or "nodeID" (defaulting handle to "main").
func parseNodeHandle(s string) (nodeID, handle string, err error) {
	if s == "" {
		return "", "", fmt.Errorf("value is required")
	}
	parts := strings.SplitN(s, ":", 2)
	nodeID = parts[0]
	if len(parts) == 2 && parts[1] != "" {
		handle = parts[1]
	} else {
		handle = "main"
	}
	return nodeID, handle, nil
}
