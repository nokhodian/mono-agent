# Feature Map: n8n

_Line refs from repository at /Users/morteza/Desktop/monoes/n8n_

## System Overview

n8n v2.12.0 is an open-source workflow automation platform (iPaaS) built as a Turbo-orchestrated monorepo of 60+ packages. Users compose workflows as directed acyclic graphs (DAGs) of nodes on a Vue 3 canvas; execution is handled by a Node.js backend that can run jobs in-process (simple mode) or distribute them across worker processes via a Bull/Redis job queue. The platform exposes a REST API (v1) for programmatic control and pushes live execution state to connected browser sessions via WebSocket or SSE. Enterprise features—RBAC, SSO, audit logs, credential sharing, license gating—are layered on top of the open-source core and gate-checked at every authorization boundary.

## Architecture Snapshot

### Tech Stack

- **Runtime:** Node.js 22, TypeScript 5
- **Web framework:** Express 5 with decorator-based controller registry (`packages/cli/src/controller.registry.ts`)
- **ORM / DB:** @n8n/typeorm (fork of TypeORM 0.3); SQLite (default) or PostgreSQL
- **Queue:** Bull 4 over Redis (ioredis)
- **Frontend:** Vue 3 + Vite + Pinia + Vue Flow (canvas)
- **Expression engine:** n8n-workflow package with tmpl/lodash-based evaluator
- **Auth:** JWT (jsonwebtoken) in HttpOnly cookies; LDAP (ldapts), SAML (samlify), OIDC (openid-client)
- **Push transport:** ws (WebSocket) or Node.js built-in SSE
- **Validation:** Zod on every request DTO
- **Binary data:** file system, S3, or in-DB base64
- **Task runner:** sandboxed child-process / Python sidecar for untrusted code execution
- **Metrics:** prom-client (Prometheus export)
- **Build / DI:** Turbo + @n8n/di (reflect-metadata IoC)

### Data Flow

```
External HTTP / Timer / UI
  → WebhookRequestHandler.handleRequest()  OR  ScheduledTaskManager cron fires
  → ActiveWorkflowManager resolves workflow + hooks
  → WorkflowRunner.run()  →  (simple) WorkflowExecute.run()  |  (queue) ScalingService.addJob()
  → [Worker] JobProcessor.processJob()  →  WorkflowExecute.run()
  → Execution stack loop: pop IExecuteData → Node.execute() → collect INodeExecutionData[]
  → runData stored in ExecutionRepository (PostgreSQL/SQLite)
  → Lifecycle hook fires pushMessage(executionFinished)
  → Push service sends JSON frame to WebSocket/SSE client
  → Frontend workflowsEE store receives event → updates canvas node status
```

### Key Entry Points

- `packages/cli/src/commands/start.ts:1` — CLI bootstrap, Express app initialization
- `packages/cli/src/commands/worker.ts:1` — Queue worker process entry point
- `packages/core/src/execution-engine/workflow-execute.ts:99` — `WorkflowExecute` — DAG execution engine
- `packages/cli/src/webhooks/webhook-request-handler.ts:38` — `handleRequest()` — all incoming webhook HTTP
- `packages/cli/src/active-workflow-manager.ts:1` — `ActiveWorkflowManager` — trigger/webhook lifecycle
- `packages/cli/src/scaling/scaling.service.ts:60` — `ScalingService.setupQueue()` — Bull queue init
- `packages/cli/src/controller.registry.ts:1` — decorator-driven Express route registration
- `packages/cli/src/push/index.ts:69` — WebSocket/SSE push server setup
- `packages/frontend/editor-ui/src/features/workflows/canvas/components/Canvas.vue:44` — Vue Flow canvas root
- `packages/@n8n/db/src/connection/db-connection.ts:20` — TypeORM connection + migration runner

## Feature Index

| ID | Feature | Category | Complexity | Key File |
|----|---------|----------|------------|----------|
| F-001 | Workflow Instantiation & DAG Construction | Core | High | `packages/core/src/execution-engine/workflow-execute.ts:123` |
| F-002 | Execution Mode & Initialization | Core | Medium | `packages/core/src/execution-engine/workflow-execute.ts:105` |
| F-003 | Execution Stack & Node Scheduling | Core | High | `packages/core/src/execution-engine/workflow-execute.ts:1489` |
| F-004 | Multi-Input Node Synchronization (Waiting Execution) | Core | High | `packages/core/src/execution-engine/workflow-execute.ts:420` |
| F-005 | Node Execution Loop & Processing | Core | High | `packages/core/src/execution-engine/workflow-execute.ts:1595` |
| F-006 | Node Retry Logic | Core | Medium | `packages/core/src/execution-engine/workflow-execute.ts:1609` |
| F-007 | Pin Data (Mocked/Cached Node Output) | Core | Low | `packages/core/src/execution-engine/workflow-execute.ts:1641` |
| F-008 | Expression Evaluation | Core | High | `packages/core/src/execution-engine/node-execution-context/execute-single-context.ts:88` |
| F-009 | Execution Status Lifecycle | Core | Medium | `packages/core/src/execution-engine/workflow-execute.ts:100` |
| F-010 | Error Handling & Node Failure Recovery | Core | High | `packages/core/src/execution-engine/workflow-execute.ts:1786` |
| F-011 | Execution Result Storage | Core | Medium | `packages/core/src/execution-engine/workflow-execute.ts:1822` |
| F-012 | Partial Execution & Destination Node | Core | High | `packages/core/src/execution-engine/workflow-execute.ts:197` |
| F-013 | Execution Timeout | Core | Low | `packages/core/src/execution-engine/workflow-execute.ts:1492` |
| F-014 | Execution Cancellation | Core | Medium | `packages/core/src/execution-engine/workflow-execute.ts:1425` |
| F-015 | Execution Lifecycle Hooks | Core | Medium | `packages/core/src/execution-engine/execution-lifecycle-hooks.ts:15` |
| F-016 | Waiting Execution State | Core | High | `packages/core/src/execution-engine/workflow-execute.ts:1291` |
| F-017 | Paired Items Tracking | Core | Medium | `packages/core/src/execution-engine/workflow-execute.ts:2592` |
| F-018 | Workflow Ready-State Validation | Core | Medium | `packages/core/src/execution-engine/workflow-execute.ts:826` |
| F-019 | Static Data Persistence | Core | Low | `packages/core/src/execution-engine/workflow-execute.ts:2413` |
| F-020 | Binary Data Conversion & Storage | Core | Medium | `packages/core/src/execution-engine/workflow-execute.ts:1716` |
| F-021 | Execution Mode-Specific Behavior | Core | Medium | `packages/core/src/execution-engine/workflow-execute.ts:1086` |
| F-022 | Webhook Registration and Persistence in Database | Trigger | Medium | `packages/cli/src/active-workflow-manager.ts:150` |
| F-023 | Live Webhook Execution (Production URL) | Trigger | High | `packages/cli/src/webhooks/live-webhooks.ts:71` |
| F-024 | Test Webhook Execution (Manual Testing URL) | Trigger | High | `packages/cli/src/webhooks/test-webhooks.ts:67` |
| F-025 | Webhook Path Routing (Static vs. Dynamic) | Trigger | Medium | `packages/cli/src/webhooks/webhook.service.ts:84` |
| F-026 | Cron-Based Scheduled Trigger Activation | Trigger | Medium | `packages/core/src/execution-engine/active-workflows.ts:141` |
| F-027 | Polling Trigger Execution | Trigger | Medium | `packages/core/src/execution-engine/active-workflows.ts:241` |
| F-028 | Trigger Node Activation and emit/emitError Callbacks | Trigger | High | `packages/core/src/execution-engine/triggers-and-pollers.ts:26` |
| F-029 | Trigger Deactivation and closeFunction Cleanup | Trigger | Medium | `packages/core/src/execution-engine/active-workflows.ts:182` |
| F-030 | Multi-Instance Webhook Activation (Leadership & Pub/Sub Coordination) | Trigger | High | `packages/cli/src/active-workflow-manager.ts:588` |
| F-031 | Workflow Activation Retry Queue with Exponential Backoff | Trigger | Medium | `packages/cli/src/active-workflow-manager.ts:811` |
| F-032 | Webhook Node Setup Methods (checkExists, create, delete) | Trigger | Medium | `packages/cli/src/webhooks/webhook.service.ts:407` |
| F-033 | Manual Trigger Execution (UI Test Button) | Trigger | Medium | `packages/cli/src/manual-execution.service.ts:29` |
| F-034 | Webhook Timeout and Cancellation for Manual Testing | Trigger | Medium | `packages/cli/src/webhooks/test-webhooks.ts:324` |
| F-035 | Waiting Webhooks for Resume-on-Webhook (Wait Node & Form) | Trigger | High | `packages/cli/src/webhooks/waiting-webhooks.ts:131` |
| F-036 | Webhook Response Modes (Synchronous, Last Node, On-Received) | Trigger | High | `packages/cli/src/webhooks/webhook-helpers.ts:193` |
| F-037 | Webhook CORS Handling | Trigger | Low | `packages/cli/src/webhooks/webhook-request-handler.ts:180` |
| F-038 | Trigger Count Tracking | Trigger | Low | `packages/cli/src/active-workflow-manager.ts:780` |
| F-039 | Execution Mode Selection (Simple vs Queue) | Queue | Medium | `packages/cli/src/commands/worker.ts:67` |
| F-040 | Bull Queue Setup and Configuration | Queue | High | `packages/cli/src/scaling/scaling.service.ts:60` |
| F-041 | Job Data Serialization and Structure | Queue | Medium | `packages/cli/src/scaling/scaling.types.ts:18` |
| F-042 | Worker Startup and Job Processing Registration | Queue | High | `packages/cli/src/commands/worker.ts:111` |
| F-043 | Job Enqueue with Priority | Queue | Medium | `packages/cli/src/scaling/scaling.service.ts:226` |
| F-044 | Worker Job Processing and Execution | Queue | High | `packages/cli/src/scaling/job-processor.ts:72` |
| F-045 | Job Concurrency Limits (Per-Worker) | Queue | Medium | `packages/cli/src/commands/worker.ts:151` |
| F-046 | Concurrency Control in Simple Mode (Production/Evaluation) | Queue | Medium | `packages/cli/src/concurrency/concurrency-control.service.ts:25` |
| F-047 | Job Failure Handling and Retry | Queue | Medium | `packages/cli/src/scaling/scaling.service.ts:139` |
| F-048 | Graceful Shutdown and Execution Cancellation | Queue | Medium | `packages/cli/src/scaling/scaling.service.ts:163` |
| F-049 | Queue Recovery (Dangling Execution Detection) | Queue | High | `packages/cli/src/scaling/scaling.service.ts:575` |
| F-050 | Queue Metrics Collection and Prometheus Export | Queue | Medium | `packages/cli/src/scaling/scaling.service.ts:524` |
| F-051 | Job Message Routing (Progress, Response, Chunk, MCP) | Queue | High | `packages/cli/src/scaling/scaling.service.ts:338` |
| F-052 | Worker Status Monitoring and Reporting | Queue | Medium | `packages/cli/src/scaling/worker-status.service.ee.ts:22` |
| F-053 | Redis Connection with Scaling/PubSub | Queue | Medium | `packages/cli/src/scaling/scaling.service.ts:60` |
| F-054 | Multi-Main Setup with PubSub Orchestration | Queue | High | `packages/cli/src/scaling/constants.ts:8` |
| F-055 | Task Runner Sandboxing | Queue | High | `packages/cli/src/commands/worker.ts:45` |
| F-056 | Controller Registration via Decorators | API | Medium | `packages/@n8n/decorators/src/controller/rest-controller.ts:6` |
| F-057 | Zod-based Request Validation with Auto-Extraction | API | Medium | `packages/cli/src/controller.registry.ts:100` |
| F-058 | Licensing Enforcement Decorator | API | Low | `packages/@n8n/decorators/src/controller/licensed.ts:7` |
| F-059 | Scope-based Access Control (Global vs Project Level) | API | Medium | `packages/@n8n/decorators/src/controller/scoped.ts:7` |
| F-060 | IP-based Rate Limiting | API | Low | `packages/cli/src/services/rate-limit.service.ts:31` |
| F-061 | User Keyed Rate Limiting | API | Low | `packages/cli/src/services/rate-limit.service.ts:65` |
| F-062 | Body Field Keyed Rate Limiting | API | Low | `packages/cli/src/services/rate-limit.service.ts:45` |
| F-063 | Authentication Middleware with MFA Support | API | High | `packages/cli/src/auth/auth.service.ts:96` |
| F-064 | CORS Header Application (Per-Route) | API | Low | `packages/cli/src/services/cors-service.ts:18` |
| F-065 | Response Envelope Wrapping (Success and Error) | API | Low | `packages/cli/src/response-helper.ts:156` |
| F-066 | WebSocket Push Connection Lifecycle | API | High | `packages/cli/src/push/websocket.push.ts:15` |
| F-067 | SSE Push Connection Lifecycle | API | Medium | `packages/cli/src/push/sse.push.ts:11` |
| F-068 | Push Backend Selection (WebSocket vs SSE) | API | Low | `packages/cli/src/push/push.config.ts:1` |
| F-069 | Push Message Types and Execution Streaming | API | Medium | `packages/@n8n/api-types/src/push/index.ts:11` |
| F-070 | Push Message Routing (Single User, Multiple Users, Broadcast) | API | High | `packages/cli/src/push/index.ts:158` |
| F-071 | Push Message Serialization and Binary Frames | API | Medium | `packages/cli/src/push/abstract.push.ts:76` |
| F-072 | Push Authentication and Origin Validation | API | Medium | `packages/cli/src/push/index.ts:93` |
| F-073 | Push Server HTTP Upgrade Handler | API | Medium | `packages/cli/src/push/index.ts:69` |
| F-074 | Push Endpoint Registration | API | Low | `packages/cli/src/push/index.ts:92` |
| F-075 | Pagination with Skip/Take Pattern | API | Low | `packages/@n8n/api-types/src/dto/pagination/pagination.dto.ts:37` |
| F-076 | Pagination with Offset/Limit Pattern | API | Low | `packages/@n8n/api-types/src/dto/pagination/pagination.dto.ts:68` |
| F-077 | API Key Authentication | API | Medium | `packages/@n8n/decorators/src/controller/route.ts:23` |
| F-078 | Middleware Method Execution | API | Low | `packages/@n8n/decorators/src/controller/middleware.ts:6` |
| F-079 | Static Router Mounting | API | Low | `packages/cli/src/controller.registry.ts:64` |
| F-080 | JWT Cookie Issuance & Validation | Auth | Medium | `packages/cli/src/auth/auth.service.ts:204` |
| F-081 | Email/Password Authentication | Auth | Medium | `packages/cli/src/auth/handlers/email.auth-handler.ts:23` |
| F-082 | Password Reset Flow | Auth | Medium | `packages/cli/src/controllers/password-reset.controller.ts:61` |
| F-083 | LDAP Authentication & Synchronization | Auth | High | `packages/cli/src/modules/ldap.ee/ldap.service.ee.ts:50` |
| F-084 | SAML 2.0 Authentication | Auth | High | `packages/cli/src/modules/sso-saml/saml.service.ee.ts:42` |
| F-085 | OIDC/OpenID Connect Authentication | Auth | High | `packages/cli/src/modules/sso-oidc/oidc.service.ee.ts:58` |
| F-086 | MFA (Multi-Factor Authentication) | Auth | High | `packages/cli/src/mfa/mfa.service.ts:15` |
| F-087 | User Signup via Invite Links | Auth | Medium | `packages/cli/src/controllers/auth.controller.ts:199` |
| F-088 | User Invitation & Email Sending | Auth | Medium | `packages/cli/src/controllers/users.controller.ts:172` |
| F-089 | Logout & Token Invalidation | Auth | Low | `packages/cli/src/controllers/auth.controller.ts:264` |
| F-090 | Role-Based Access Control (RBAC) | Auth | High | `packages/@n8n/db/src/entities/role.ts:1` |
| F-091 | Permission Scopes & Global Scope Checks | Auth | Medium | `packages/cli/src/controllers/users.controller.ts:111` |
| F-092 | Credential Sharing with Role-Based Access | Auth | Medium | `packages/@n8n/db/src/entities/shared-credentials.ts:1` |
| F-093 | Workflow Sharing with Role-Based Access | Auth | Medium | `packages/@n8n/db/src/entities/shared-workflow.ts:1` |
| F-094 | Project Membership & Access Control | Auth | High | `packages/cli/src/services/user.service.ts:179` |
| F-095 | License Feature Gates | Auth | Medium | `packages/cli/src/license.ts:36` |
| F-096 | API Key Authentication & Scope Validation | Auth | Medium | `packages/cli/src/controllers/api-keys.controller.ts:42` |
| F-097 | Instance Owner Setup | Auth | Medium | `packages/@n8n/db/src/constants.ts:52` |
| F-098 | Browser ID Session Hijacking Prevention | Auth | Medium | `packages/cli/src/auth/auth.service.ts:168` |
| F-099 | Authentication Method Switching (Email ↔ LDAP ↔ SAML ↔ OIDC) | Auth | Medium | `packages/cli/src/sso.ee/sso-helpers.ts:14` |
| F-100 | SSO Just-In-Time (JIT) Provisioning | Auth | High | `packages/@n8n/config/src/configs/sso.config.ts:57` |
| F-101 | API Key Scopes (License-Gated) | Auth | Medium | `packages/cli/src/controllers/api-keys.controller.ts:125` |
| F-102 | Instance Owner & Admin Role Distinctions | Auth | Medium | `packages/cli/src/controllers/users.controller.ts:341` |
| F-103 | Database Connection Initialization with TypeORM | Data | Medium | `packages/@n8n/db/src/connection/db-connection.ts:20` |
| F-104 | Database Configuration with SQLite and PostgreSQL Support | Data | Medium | `packages/@n8n/db/src/connection/db-connection-options.ts:16` |
| F-105 | Custom Migration DSL with Database-Agnostic API | Data | High | `packages/@n8n/db/src/migrations/migration-helpers.ts:203` |
| F-106 | Migration Types and Interfaces | Data | Low | `packages/@n8n/db/src/migrations/migration-types.ts:1` |
| F-107 | CredentialsEntity with Encryption, Sharing, and Resolver Support | Data | Medium | `packages/@n8n/db/src/entities/credentials-entity.ts:8` |
| F-108 | Credential Decryption and Redaction in Service Layer | Data | High | `packages/cli/src/credentials/credentials.service.ts:637` |
| F-109 | Credentials Repository with ListQuery Filtering and Sharing Subqueries | Data | High | `packages/@n8n/db/src/repositories/credentials.repository.ts:12` |
| F-110 | SharedCredentials Entity with Project-Based Access Control | Data | Medium | `packages/@n8n/db/src/entities/shared-credentials.ts:8` |
| F-111 | Shared Credentials Repository with Subquery-Based Permission Checks | Data | High | `packages/@n8n/db/src/repositories/shared-credentials.repository.ts:10` |
| F-112 | WorkflowEntity with Versioning, Tags, Sharing, and Pin Data | Data | High | `packages/@n8n/db/src/entities/workflow-entity.ts:25` |
| F-113 | ExecutionEntity with Status, Retry Chain, and Flexible Data Storage | Data | High | `packages/@n8n/db/src/entities/execution-entity.ts:25` |
| F-114 | ExecutionData with Workflow Definition and Pin Data | Data | Medium | `packages/@n8n/db/src/entities/execution-data.ts:9` |
| F-115 | User Entity with Email, Password, MFA, and Auth Identities | Data | Medium | `packages/@n8n/db/src/entities/user.ts:29` |
| F-116 | AuthIdentity Entity for OAuth/LDAP/SSO Integration | Data | Medium | `packages/@n8n/db/src/entities/auth-identity.ts:8` |
| F-117 | ApiKey Entity with Scopes and Audience | Data | Low | `packages/@n8n/db/src/entities/api-key.ts:8` |
| F-118 | TagEntity with ManyToMany Workflow and Folder Mapping | Data | Low | `packages/@n8n/db/src/entities/tag-entity.ts:9` |
| F-119 | WebhookEntity for Webhook HTTP Routing | Data | Medium | `packages/@n8n/db/src/entities/webhook-entity.ts:4` |
| F-120 | ProjectEntity with Team/Personal Distinction and Nested Access Control | Data | Medium | `packages/@n8n/db/src/entities/project.ts:11` |
| F-121 | SettingsEntity for Key-Value Configuration Store | Data | Low | `packages/@n8n/db/src/entities/settings.ts:10` |
| F-122 | Variables Entity with Project Scoping for Environment Variables | Data | Low | `packages/@n8n/db/src/entities/variables.ts:6` |
| F-123 | Abstract Entity Base Classes with ID Generation and Timestamps | Data | Low | `packages/@n8n/db/src/entities/abstract-entity.ts:14` |
| F-124 | Credential Encryption/Decryption Integration with n8n-core | Data | High | `packages/cli/src/credentials/credentials.service.ts:637` |
| F-125 | Credential Update Workflow with OAuth Token Preservation | Data | High | `packages/cli/src/credentials/credentials.service.ts:558` |
| F-126 | Credential Redaction for API Responses | Data | Medium | `packages/cli/src/credentials/credentials.service.ts:754` |
| F-127 | ExecutionRepository with Complex Filtering, Status Updates, and Batch Operations | Data | High | `packages/@n8n/db/src/repositories/execution.repository.ts:1` |
| F-128 | WorkflowRepository with Complex Permission Queries and Folder/Tag Navigation | Data | High | `packages/@n8n/db/src/repositories/workflow.repository.ts:57` |
| F-129 | ExecutionRepository Retry Chain and Status Update Logic | Data | High | `packages/@n8n/db/src/repositories/execution.repository.ts:66` |
| F-130 | Shared Workflow Entity and Repository for Multi-Project Workflow Sharing | Data | Medium | `—` |
| F-131 | Project Relationship Entity and Role-Based Access Control | Data | Medium | `—` |
| F-132 | Workflow Canvas Rendering with Vue Flow | Frontend | High | `packages/frontend/editor-ui/src/features/workflows/canvas/components/Canvas.vue:44` |
| F-133 | Node Drag-and-Drop onto Canvas | Frontend | Medium | `—` |
| F-134 | Node Connection (Edge) Creation via Handles | Frontend | High | `—` |
| F-135 | Node Configuration Panel (NDV - Node Details View) | Frontend | High | `—` |
| F-136 | Expression Editor (CodeMirror 6 Integration) | Frontend | High | `—` |
| F-137 | Node Execution Results Overlay | Frontend | Medium | `—` |
| F-138 | Workflow Run Button with Trigger Selection | Frontend | Medium | `—` |
| F-139 | Manual Node Execution (Run Button on Toolbar) | Frontend | Medium | `—` |
| F-140 | Stop Workflow / Stop Webhook Execution | Frontend | Medium | `—` |
| F-141 | Workflow Save with Auto-Save | Frontend | Medium | `—` |
| F-142 | Undo / Redo History | Frontend | High | `—` |
| F-143 | Copy / Cut / Paste Nodes | Frontend | Medium | `—` |
| F-144 | Keyboard Shortcuts & Navigation | Frontend | Medium | `—` |
| F-145 | Node Search & Creation (Node Creator Panel) | Frontend | High | `—` |
| F-146 | Node Credentials Selection & Management | Frontend | High | `—` |
| F-147 | Error Indicators on Nodes | Frontend | Low | `—` |
| F-148 | Execution History Sidebar | Frontend | Medium | `—` |
| F-149 | Node Renaming (Inline Rename) | Frontend | Low | `—` |
| F-150 | Node Disable/Enable Toggle | Frontend | Low | `—` |
| F-151 | Sticky Notes | Frontend | Low | `—` |
| F-152 | Zoom & Pan Controls | Frontend | Low | `—` |
| F-153 | Node Selection (Single & Multi-Select) | Frontend | Medium | `—` |
| F-154 | Sub-Workflow Opening | Frontend | Low | `—` |
| F-155 | Canvas Layout / Auto-Arrange (Tidy Up) | Frontend | Medium | `—` |
| F-156 | Viewport Auto-Adjustment | Frontend | Low | `—` |
| F-157 | Context Menu (Right-Click Node Actions) | Frontend | Medium | `—` |
| F-158 | Connection Line During Drag (Visual Feedback) | Frontend | Low | `—` |
| F-159 | Experimental Embedded NDV (Zoom-Focused Mode) | Frontend | Medium | `—` |
| F-160 | Node Type Icons & Display Metadata | Frontend | Low | `—` |
## Features

### F-001: Workflow Instantiation & DAG Construction
**Category:** Core
**Complexity:** High

#### What it does
Creates a Workflow object from JSON definition, resolves node types, and establishes directed connections between nodes. The workflow is represented as a DAG with nodes as vertices and connections as edges.

#### Specification
- Workflow loaded from database as JSON containing nodes array and connections structure
- Node types resolved dynamically by name and version from node type registry
- Connections indexed bidirectionally: `connectionsBySourceNode` and `connectionsByDestinationNode`
- Nodes can be disabled (flag `node.disabled === true`) and are filtered from execution
- Start nodes identified via `workflow.getStartNode()` - nodes with no incoming connections
- Supports legacy execution order (v1) vs modern order tracked in `workflow.settings.executionOrder`

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:123-187 — `run()` method`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:99-138 — WorkflowExecute class initialization and run method
- packages/@n8n/db/src/entities/execution-data.ts:1-40 — Execution data entity with workflowData JSON column

#### Dependencies
- Depends on: none
- External: n8n-workflow (Workflow, INode, IConnection types)

#### Porting Notes
Workflow is immutable during execution. Node typeVersions must be resolved before execution begins. The DAG structure is rebuilt during partial execution with DirectedGraph utility.

---

### F-002: Execution Mode & Initialization
**Category:** Core
**Complexity:** Medium

#### What it does
Initializes workflow execution with a specific mode (manual, trigger, webhook, etc.) and establishes execution context including additional data, hooks, and abort signals.

#### Specification
- Execution modes: 'manual', 'trigger', 'webhook', 'internal', 'test'
- Mode determines execution behavior (e.g., poll nodes run their poll function in manual mode only)
- Execution context establishes via `establishExecutionContext()` before node execution begins
- AbortController created for cancellation (line 102)
- WorkflowExecuteMode type parameter required on all execution paths
- Execution lifecycle hooks registered via `additionalData.hooks`

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:105-110 — constructor`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:99-110 — Class initialization with mode
- packages/core/src/execution-engine/execution-context.ts:1-50 — establishExecutionContext function

#### Dependencies
- Depends on: F-001
- External: n8n-workflow, PCancelable library for cancellation

#### Porting Notes
Mode cannot change during execution. AbortSignal is passed to all node executions and must be checked for cancellation.

---

### F-003: Execution Stack & Node Scheduling
**Category:** Core
**Complexity:** High

#### What it does
Maintains a stack of nodes to be executed in order, allowing nodes to be added/removed dynamically as previous nodes complete. Controls execution ordering and ensures dependencies are resolved.

#### Specification
- Stack stored as `this.runExecutionData.executionData.nodeExecutionStack` array
- Each stack entry is `IExecuteData` containing node, input data, and source metadata
- Nodes popped from stack and executed one at a time (line 1509)
- When node completes, child nodes added to stack via `addNodeToBeExecuted()` (line 406)
- Stack enqueue strategy (push vs unshift) determined by execution order version (line 417)
- Legacy v1 execution order uses position-based sorting; v2+ uses different ordering
- Loop continues while stack has items (line 1489-1491)
- Execution can be paused if node produces EngineRequest (async request/response pattern)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1489-1510 — Main execution loop`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:406-817 — addNodeToBeExecuted method manages stack and waiting execution
- packages/core/src/execution-engine/workflow-execute.ts:1987-2080 — Adding child nodes to execution stack after completion

#### Dependencies
- Depends on: F-001, F-002
- External: n8n-workflow types

#### Porting Notes
The waiting execution mechanism is complex - nodes with multiple inputs must wait for all input data before executing. Stack operations must maintain index consistency across iterations.

---

### F-004: Multi-Input Node Synchronization (Waiting Execution)
**Category:** Core
**Complexity:** High

#### What it does
Handles nodes with multiple input connections by pausing their execution until all required inputs have data. Maintains partial data in a "waiting" state and merges them when all inputs arrive.

#### Specification
- Triggered when node has `numberOfInputs > 1` (line 424)
- Waiting data stored in `executionData.waitingExecution[nodeName][waitingNodeIndex]` keyed by connection index
- Source tracking stored in `waitingExecutionSource[nodeName][waitingNodeIndex]` to maintain lineage
- Node only executed when ALL inputs contain data (line 506-524 check)
- Can have multiple waiting entries for same node if inputs arrive at different times
- Entry created fresh if no data yet exists for node (line 436-438)
- Source information includes `previousNode`, `previousNodeOutput`, `previousNodeRun` (line 500-502)
- Null inputs treated as "no data received yet" (distinct from empty arrays)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:420-525 — Multi-input handling in addNodeToBeExecuted`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:387-403 — prepareWaitingToExecution initializes structure
- packages/core/src/execution-engine/workflow-execute.ts:2092-2249 — Waiting node execution logic at end of execution loop

#### Dependencies
- Depends on: F-003
- External: None specific

#### Porting Notes
Critical for merge nodes and other multi-input nodes. The waiting state is ephemeral - data is moved to execution stack once complete. No persistence to database while waiting.

---

### F-005: Node Execution Loop & Processing
**Category:** Core
**Complexity:** High

#### What it does
Executes a single node by popping from execution stack, preparing input data, calling node's execute function, handling results, and queueing dependent nodes.

#### Specification
- Current node popped from stack: `executionData = nodeExecutionStack.shift()` (line 1509)
- Run index tracked per node - increments with each execution pass (line 1565-1570)
- Execution prevented if node not in runNodeFilter (line 1580-1587)
- Input data validated with `ensureInputData()` - missing inputs cause node to be re-queued (line 1589)
- PairedItem information updated to track input/output relationships (line 1531-1558)
- `nodeExecuteBefore` hook fired before execution (line 1607)
- Disabled nodes skip execution and pass through first input unchanged (line 911-920)
- Node types: execute, trigger, poll, webhook, declarative
- Each node type has specific execution path with different input handling

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1595-1668 — Node execution dispatch`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:1181-1280 — runNode method dispatches to execute/trigger/poll/webhook handlers
- packages/core/src/execution-engine/workflow-execute.ts:1004-1073 — executeNode invokes node's execute function via ExecuteContext

#### Dependencies
- Depends on: F-001, F-002, F-003, F-004
- External: Node's execute/trigger/poll functions, ExecuteContext, NodeHelpers

#### Porting Notes
Run index is crucial for tracking multiple executions of same node in branching scenarios. PairedItem tracking enables error output and item mapping features.

---

### F-006: Node Retry Logic
**Category:** Core
**Complexity:** Medium

#### What it does
Automatically retries node execution on failure with configurable retry count and wait interval between attempts.

#### Specification
- Retry enabled by `executionData.node.retryOnFail === true` (line 1610)
- Max retries: `maxTries = Math.min(5, Math.max(2, executionData.node.maxTries || 3))` (line 1612)
- Default max tries is 3, clamped between 2 and 5
- Wait between retries: `Math.min(5000, Math.max(0, executionData.node.waitBetweenTries || 1000))` milliseconds (line 1618-1620)
- Default wait is 1000ms, clamped between 0 and 5000ms
- Retry loop iterates from 0 to maxTries (line 1624)
- On retry, `executionError` reset to undefined (line 1627-1628)
- Retry triggered if first output item contains error: `runNodeData.data?.[0]?.[0]?.json?.error !== undefined` (line 1681)
- Loop breaks once node succeeds (no error) or max attempts reached (line 1785)
- Only retries on node-level errors, not on EngineRequest returns

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1609-1700 — Retry configuration and loop`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:1609-1700 — Full retry implementation

#### Dependencies
- Depends on: F-005
- External: sleep() from n8n-workflow

#### Porting Notes
Retry only looks at first output's first item for error detection. Does not affect EngineRequest (async node) pattern. Wait time uses setTimeout blocking entire execution (marked as TODO for improvement at line 1630-1631).

---

### F-007: Pin Data (Mocked/Cached Node Output)
**Category:** Core
**Complexity:** Low

#### What it does
Allows execution to use pre-defined "pinned" data for specific nodes instead of executing them, useful for development and testing.

#### Specification
- Pin data stored in `runExecutionData.resultData.pinData: IPinData` (line 1641)
- Format: `{[nodeName]: INodeExecutionData[]}` - contains full output data for node
- If node has pin data and is not disabled, use pinned output instead of executing (line 1643-1646)
- Pin data takes precedence over node execution
- Pin data passed as parameter to `run()` method (line 127)
- Available nodes with pin data included in readiness checks via `pinDataNodeNames` parameter (line 831-877)
- Pin data preserved across partial executions (line 291)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1641-1646 — Pin data usage`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:86-97 — RunWorkflowOptions interface with pinData
- packages/core/src/execution-engine/workflow-execute.ts:826-891 — checkReadyForExecution validates pinned nodes
- packages/@n8n/db/src/entities/execution-data.ts:24-25 — Pin data stored in ExecutionData entity

#### Dependencies
- Depends on: F-005
- External: n8n-workflow

#### Porting Notes
Pin data is completely bypasses node execution. Used heavily in UI for manual run testing. Disabled nodes ignore pin data (line 1643).

---

### F-008: Expression Evaluation
**Category:** Core
**Complexity:** High

#### What it does
Evaluates dynamic expressions in node parameters using the workflow data proxy, allowing nodes to reference data from upstream nodes.

#### Specification
- Expressions evaluated lazily in node context via `evaluateExpression()` method
- Evaluation happens at single-item level in ExecuteSingleContext (line 88-89)
- WorkflowDataProxy provides access to `$json`, `$input`, `$node`, `$env`, `$secrets`, `$credentials`
- Expressions run in sandboxed V8 context with custom timeout
- Parameter types resolved with `getNodeParameter()` supporting expressions, defaults, and validation
- Item index provided to expression context for accessing current item in arrays
- Source overwrite information preserved for paired items in tool execution (line 1533-1548)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/node-execution-context/execute-single-context.ts:88-90 — evaluateExpression`
**Key files:**
- packages/core/src/execution-engine/node-execution-context/base-execute-context.ts — Base context with expression evaluation
- packages/core/src/execution-engine/node-execution-context/execute-single-context.ts:26-86 — Single-item context for expression evaluation

#### Dependencies
- Depends on: F-001
- External: n8n-workflow expression evaluator, WorkflowDataProxy

#### Porting Notes
Expressions are evaluated per-item, so context changes with itemIndex. Source overwrite tracking needed for AI agent tool execution.

---

### F-009: Execution Status Lifecycle
**Category:** Core
**Complexity:** Medium

#### What it does
Tracks execution status through various states from initialization through completion, stored in database for querying and UI updates.

#### Specification
- Status enum: 'new', 'running', 'waiting', 'success', 'error', 'canceled' (from ExecutionStatus)
- Initialized as 'new' in constructor (line 100)
- Set to 'running' when execution begins (line 131, 282, 1341)
- Individual task status: 'running', 'success', 'waiting', 'error' (line 1830, 1837, 2656-2657)
- Status 'waiting' set on `runExecutionData.waitTill` being defined (line 1830, 2402-2407)
- Status 'canceled' set when abort signal triggered or timeout exceeded (line 1431, 1496, 2398)
- Final status determined in `processSuccessExecution()` (line 2389-2411)
- Execution entity status column type: 'varchar' (execution-entity.ts:54)
- Task data status tracks execution state per node execution

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:100-100 — status field; 2389-2411 — Final status determination`
**Key files:**
- packages/@n8n/db/src/entities/execution-entity.ts:54 — ExecutionStatus column definition
- packages/core/src/execution-engine/workflow-execute.ts:2382-2411 — processSuccessExecution status finalization

#### Dependencies
- Depends on: F-005, F-011
- External: ExecutionStatus from n8n-workflow

#### Porting Notes
Status transitions are one-directional forward (new→running→success/error/waiting/canceled). Task status differs from execution status - individual tasks can be 'running' while execution is complete.

---

### F-010: Error Handling & Node Failure Recovery
**Category:** Core
**Complexity:** High

#### What it does
Captures node execution errors, determines recovery strategy (continue, stop, or alternate output), and stores error information for logging.

#### Specification
- Errors caught in try-catch wrapping node execution (line 1786)
- Error types: NodeApiError, NodeOperationError, ApplicationError, generic Error
- Error wrapped as `ExecutionBaseError` with message and stack (line 1810)
- Three recovery strategies:
- 1. `continueOnFail === true`: Pass input through as output, execution continues (line 1854)
- 2. `onError === 'continueRegularOutput'`: Same as continueOnFail (line 1855-1856)
- 3. `onError === 'continueErrorOutput'`: Route error to separate error output (line 1729, 2474-2571)
- Default behavior (no recovery): Stop execution, add node back to stack for retry attempt (line 1895)
- Error reported to Sentry if not ApplicationError wrapper (line 1797-1805)
- Error stored in task data (line 1836)
- For merged items with errors, separate output path created to avoid mixing success/error items

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1786-1906 — Error catch block and recovery`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:1786-1906 — Error handling and continuation logic
- packages/core/src/execution-engine/workflow-execute.ts:2474-2571 — handleNodeErrorOutput routes errors to dedicated output

#### Dependencies
- Depends on: F-005
- External: ErrorReporter service, error types from n8n-workflow

#### Porting Notes
Error output handling complex - uses pairedItem data to map which input items caused errors. Unhandled wrapped errors in ApplicationError class are properly reported to Sentry.

---

### F-011: Execution Result Storage
**Category:** Core
**Complexity:** Medium

#### What it does
Collects output data from all node executions and stores it in `runData` keyed by node name with execution metadata and timing.

#### Specification
- Results stored in `runExecutionData.resultData.runData: IRunData` (line 1822-1823)
- Structure: `{[nodeName]: ITaskData[]}` where array index is run index
- TaskData contains: executionIndex, startTime, executionTime, data, source, metadata, executionStatus, error (line 1826-1833)
- Execution time calculated as delta from startTime (line 1828)
- Metadata moved from temp location to final runData at end (line 318-340)
- Source tracks connection lineage: `{previousNode, previousNodeOutput, previousNodeRun}` (line 1518)
- Data stored as `ITaskDataConnections` with 'main' output (line 1930-1931)
- Run index per-node tracks how many times that node executed (line 1568-1570)
- Dynamic credentials flag tracked per execution (line 1831-1832)
- Hints accumulated from node execution (line 1725-1726)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1822-1957 — Building and storing task data`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:318-340 — moveNodeMetadata moves temp metadata to final location
- packages/core/src/execution-engine/workflow-execute.ts:1826-1833 — TaskData structure creation

#### Dependencies
- Depends on: F-005
- External: n8n-workflow types

#### Porting Notes
Run index is critical for branching scenarios. Metadata is collected during execution in temp location then moved to final spot. PairedItem information embedded in output items.

---

### F-012: Partial Execution & Destination Node
**Category:** Core
**Complexity:** High

#### What it does
Executes only a subset of workflow from a trigger/parent node to a specific destination node, used in UI testing and resuming from checkpoints.

#### Specification
- Destination node specified via `IDestinationNode` parameter with `nodeName` and execution `mode` (line 88-89)
- Two destination modes: 'inclusive' (execute destination) and 'exclusive' (execute only parents)
- Run filter created from destination node's parent nodes (line 142-154)
- Non-filtered nodes skipped even if on execution stack (line 1580-1587)
- Partial workflow uses `runPartialWorkflow2()` method (line 197-305)
- Finds trigger node for partial execution via `findTriggerForPartialExecution()` (line 234)
- Constructs subgraph containing only trigger→destination path via `findSubgraph()` (line 258)
- Handles cycles in subgraph via `handleCycles()` (line 268)
- Recreates execution stack from previous run data (line 274-275)
- Dirty nodes (changed nodes) have their run data cleaned (line 263, 271)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:197-305 — runPartialWorkflow2`
**Key files:**
- packages/core/src/execution-engine/partial-execution-utils/directed-graph.ts:1-50 — DirectedGraph for subgraph construction
- packages/core/src/execution-engine/partial-execution-utils/find-subgraph.ts — Subgraph extraction
- packages/core/src/execution-engine/partial-execution-utils/recreate-node-execution-stack.ts — Stack recreation from previous runs

#### Dependencies
- Depends on: F-001, F-003, F-005
- External: Partial execution utilities (DirectedGraph, findSubgraph, etc.)

#### Porting Notes
Partial execution is complex - must preserve previous node execution data while restarting from destination. Cycle detection necessary to prevent infinite loops in subgraph extraction.

---

### F-013: Execution Timeout
**Category:** Core
**Complexity:** Low

#### What it does
Cancels workflow execution if total time exceeds configured timeout, preventing runaway executions.

#### Specification
- Timeout set via `additionalData.executionTimeoutTimestamp` (line 1493-1494)
- Timestamp is absolute deadline (milliseconds since epoch)
- Checked at start of each execution loop iteration
- Triggers when `Date.now() >= executionTimeoutTimestamp` (line 1494)
- Status set to 'canceled' and `timedOut` flag set (line 1496-1497)
- Generates `TimeoutExecutionCancelledError` on completion (line 2260)
- Distinct from manual cancellation (ManualExecutionCancelledError)
- Execution loop exits gracefully (line 1500-1502)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1492-1498 — Timeout check`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:102-103 — timedOut flag
- packages/core/src/execution-engine/workflow-execute.ts:2255-2263 — Timeout error handling

#### Dependencies
- Depends on: F-005
- External: TimeoutExecutionCancelledError from n8n-workflow

#### Porting Notes
Timeout is checked only at loop iteration boundaries, not during node execution. Long-running nodes can exceed timeout without immediate cancellation. AbortSignal is not automatically aborted on timeout, must be handled explicitly.

---

### F-014: Execution Cancellation
**Category:** Core
**Complexity:** Medium

#### What it does
Allows external cancellation of running workflows via AbortController, propagating cancellation signal to all nodes.

#### Specification
- AbortController created at instance level (line 102)
- Cancellation triggered by calling `.cancel()` on PCancelable promise (implicit)
- `onCancel` handler sets status to 'canceled' and aborts controller (line 1430-1433)
- Abort signal passed to all node executions (line 1675)
- AbortSignal checked by nodes during long-running operations
- Canceled executions don't run `workflowExecuteAfter` hook if aborted (line 2453)
- Task statuses updated to 'canceled' for any 'running' tasks (line 2652-2660)
- PCancelable ensures promise cannot resolve after cancellation (line 2453-2458)
- MaxListeners set to Infinity to allow many nodes to listen to signal (line 1427)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1425-1436 — Cancellation setup; 2652-2661 — Cancelation status update`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:102-103 — AbortController initialization
- packages/core/src/execution-engine/workflow-execute.ts:2663-2665 — isCancelled getter

#### Dependencies
- Depends on: F-005
- External: PCancelable library, n8n-workflow error classes

#### Porting Notes
Cancellation is cooperative - nodes must check AbortSignal. MaxListeners warning suppressed to prevent spurious warnings. isCancelled flag prevents hook execution for aborted workflows.

---

### F-015: Execution Lifecycle Hooks
**Category:** Core
**Complexity:** Medium

#### What it does
Provides extension points throughout workflow execution lifecycle for plugins/enterprise features to intercept and react to execution events.

#### Specification
- Hooks are stored in `ExecutionLifecycleHooks` class with typed handler arrays (line 92-101)
- Hook types: nodeExecuteBefore, nodeExecuteAfter, workflowExecuteBefore, workflowExecuteResume, workflowExecuteAfter, sendResponse, sendChunk, nodeFetchedData
- Handlers added via `hooks.addHandler(hookName, ...handlers)` (line 109-115)
- Executed via `hooks.runHook(hookName, parameters)` (line 117-131)
- All hook handlers awaited sequentially (line 124)
- Hooks receive context-specific data (node name, task data, execution data)
- workflowExecuteAfter receives full run data and new static data if changed (line 2454-2457)
- sendChunk hook for streaming responses with structured chunks
- nodeFetchedData hook for webhook/http request completion tracking

#### Implementation
**Entry point:** `packages/core/src/execution-engine/execution-lifecycle-hooks.ts:15-131 — Hook definitions and execution`
**Key files:**
- packages/core/src/execution-engine/execution-lifecycle-hooks.ts — Complete hook system
- packages/core/src/execution-engine/workflow-execute.ts:1450-1452 — workflowExecuteBefore vs workflowExecuteResume
- packages/core/src/execution-engine/workflow-execute.ts:1607, 1898, 1960, 1975, 2086 — Hook invocations

#### Dependencies
- Depends on: F-005
- External: n8n-workflow types (IRun, ITaskData, IRunExecutionData, etc.)

#### Porting Notes
Hooks are executed sequentially, not in parallel. Hook failures don't stop execution (caught and logged separately at line 2293-2295). sendChunk hook for streaming requires special error type formatting (line 1840-1851).

---

### F-016: Waiting Execution State
**Category:** Core
**Complexity:** High

#### What it does
Pauses workflow execution when a node sets `waitTill` timestamp, resuming only after the specified time.

#### Specification
- Wait timestamp set via `runExecutionData.waitTill` (line 1292)
- When set, execution status changes to 'waiting' (line 2407)
- Full execution data returned with `waitTill` field preserved (line 2447)
- On resume, `handleWaitingState()` called (line 1413):
- Clears `waitTill` flag
- Disables the node that set wait to prevent re-execution
- Removes last execution run to avoid duplicate execution record
- Waiting breaks execution loop gracefully (line 1969)
- Resume via separate execution with `restartExecutionId` flag (line 1449-1452)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1291-1309 — handleWaitingState; 2402-2407, 2446-2447 — Waiting status`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:1959-1970 — Breaking execution on wait
- packages/@n8n/db/src/entities/execution-entity.ts:79-80 — waitTill column definition

#### Dependencies
- Depends on: F-005
- External: n8n-workflow types

#### Porting Notes
Waiting is database-level concept - execution is persisted and resumed later. Node must be disabled on resume to prevent infinite re-execution. Used for delay nodes and webhook/trigger waits.

---

### F-017: Paired Items Tracking
**Category:** Core
**Complexity:** Medium

#### What it does
Tracks relationship between input and output items through node execution, enabling error item routing and data lineage.

#### Specification
- PairedItem stored in each `INodeExecutionData` as metadata
- Structure: `{item: number, input?: number, sourceOverwrite?: ...}` (line 1543-1548)
- Auto-populated if missing during node execution (line 2592-2650)
- Three auto-assign scenarios:
- 1. Single input → all outputs paired to input[0] (line 2622-2627)
- 2. Matching counts → output[i] paired to input[i] (line 2628-2634)
- 3. Multiple→single → output paired to input[0] (line 2635-2639)
- For error output, traces back to original input item via sourceOverwrite (line 1536-1548)
- Source overwrite preserves paired item info through tool execution (line 1533-1548)
- Used in error handling to route errors to correct output branch (line 2544-2560)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:2592-2650 — assignPairedItems`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:1522-1562 — PairedItem update logic
- packages/core/src/execution-engine/workflow-execute.ts:2474-2571 — PairedItem usage in error output routing

#### Dependencies
- Depends on: F-005
- External: n8n-workflow types

#### Porting Notes
PairedItem tracking crucial for multi-item processing. SourceOverwrite field specific to tool execution in AI agents. Auto-assignment fails silently if scenarios don't match - must validate manually.

---

### F-018: Workflow Ready-State Validation
**Category:** Core
**Complexity:** Medium

#### What it does
Checks if workflow can be executed by validating node parameters, detecting missing required fields, and ensuring all referenced nodes exist.

#### Specification
- Validation occurs before execution via `checkReadyForExecution()` (line 1311-1335)
- Checks nodes from destinationNode parents or specified startNode children (line 837-849)
- Skips disabled nodes and special ToolExecutor node added dynamically (line 861-862, 855-859)
- Per-node validation via `NodeHelpers.getNodeParametersIssues()` (line 873-878)
- Returns `IWorkflowIssues` object if problems found, null if OK
- Issues keyed by node name with detailed issue types (line 882)
- Includes pin data nodes in checks (line 877)
- Throws `WorkflowHasIssuesError` if validation fails (line 1333)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:826-891 — checkReadyForExecution`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:1311-1335 — checkForWorkflowIssues called during execution

#### Dependencies
- Depends on: F-001
- External: NodeHelpers from n8n-workflow

#### Porting Notes
Validation does not check credentials - only parameter structure. ToolExecutor is special case handled at line 855-859. Issues check runs early to fail fast.

---

### F-019: Static Data Persistence
**Category:** Core
**Complexity:** Low

#### What it does
Allows workflows to maintain state between executions via `workflow.staticData` object, with dirty flag tracking if updates occurred.

#### Specification
- Static data accessed via `workflow.staticData` (line 2416)
- Dirty flag: `staticData.__dataChanged` (line 2416, 2284)
- If dirty flag set, static data serialized as part of execution result (line 2417-2418)
- Returned as `newStaticData` in hook (line 2456)
- Used for credential caching and workflow state (e.g., OAuth tokens)
- Separate from execution data - persists across multiple executions

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:2413-2420 — Static data handling`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:2273-2287 — Error path static data
- packages/core/src/execution-engine/workflow-execute.ts:2454-2457 — Hook parameter

#### Dependencies
- Depends on: F-001
- External: n8n-workflow Workflow class

#### Porting Notes
Static data is mutable during execution. Dirty flag must be explicitly set by node code. Not persisted if no flag change detected.

---

### F-020: Binary Data Conversion & Storage
**Category:** Core
**Complexity:** Medium

#### What it does
Converts binary data format based on workflow binary mode setting, determining whether binaries are stored inline, as file references, or in S3.

#### Specification
- Binary mode setting: `workflow.settings.binaryMode` (line 1720)
- Conversion occurs after node execution via `convertBinaryData()` (line 1716-1721)
- Takes workflow ID, execution ID, run node data, and mode (line 1717-1720)
- Produces converted output maintaining data structure
- Supports inline, file reference, and S3 storage modes
- Applied to all execution result data before storage

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1716-1721 — Binary conversion call`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:1716-1721 — Conversion invocation
- packages/core/src/utils/convert-binary-data.ts — Conversion logic

#### Dependencies
- Depends on: F-005
- External: convertBinaryData utility, storage adapters

#### Porting Notes
Conversion is post-execution transformation. Mode setting affects entire execution run. Binary data already in correct format passes through unchanged.

---

### F-021: Execution Mode-Specific Behavior
**Category:** Core
**Complexity:** Medium

#### What it does
Alters execution behavior based on the execution mode (manual vs trigger vs webhook), with different handling for certain node types.

#### Specification
- Poll nodes: In manual mode, call node's `poll()` function; other modes pass input data through (line 1086-1092)
- Trigger nodes: Have special execution path with different input handling
- Webhook nodes: Pass data through in declarative nodes, otherwise execute normally (line 1260-1266)
- Execution context mode passed to all node execution (line 1024)
- Mode used in expression evaluation for `getSimpleParameterValue()` (line 2127)
- Different hook behavior based on mode:
- If `restartExecutionId` set, fires `workflowExecuteResume` instead of `workflowExecuteBefore` (line 1449-1453)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/workflow-execute.ts:1086-1093 — Poll mode handling; 1239-1242 — Mode-based dispatch`
**Key files:**
- packages/core/src/execution-engine/workflow-execute.ts:1086-1093 — Poll node mode handling
- packages/core/src/execution-engine/workflow-execute.ts:1449-1453 — Hook selection based on mode

#### Dependencies
- Depends on: F-002, F-005
- External: Node type descriptors

#### Porting Notes
Mode affects fundamental execution path - cannot change during execution. Poll/trigger/webhook nodes have completely different behavior based on mode.

---

### F-022: Webhook Registration and Persistence in Database
**Category:** Trigger
**Complexity:** Medium

#### What it does
Workflows define webhooks which get stored in the `webhook_entity` table on activation. Webhooks are identified by method (GET, POST, etc.), path, and optional webhookId for dynamic paths. The system supports both static webhooks (e.g., `/webhook-uuid`) and dynamic webhooks with path parameters (e.g., `/webhook-uuid/user/:id/posts`).

#### Specification
- Webhooks are extracted from workflow nodes during activation via `WebhookHelpers.getWorkflowWebhooks()` at activation time:1
- Static webhook paths have no dynamic segments (colons), dynamic paths include `:paramName` style segments:2
- Webhook entity has composite primary key: `webhookPath` + `method`, indexed on (`webhookId`, `method`, `pathLength`)
- Dynamic webhooks require `webhookId` (UUID) prefix in path; system extracts static segments for matching
- Webhooks created at `ActiveWorkflowManager.addWebhooks()` invocation line 150–234
- Invalid duplicate webhooks throw `WebhookPathTakenError` at line 216
- Trailing slashes are normalized (stripped) during registration at lines 174–179

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/webhook-entity.ts:1-57` — WebhookEntity definition with cache key, path segment parsing, dynamic detection`
**Key files:**
- `packages/@n8n/db/src/entities/webhook-entity.ts:1-57` — WebhookEntity definition with cache key, path segment parsing, dynamic detection
- `packages/cli/src/webhooks/webhook.service.ts:128-138` — `storeWebhook()` upserts to DB and cache
- `packages/cli/src/webhooks/webhook-helpers.ts:135-173` — `getWorkflowWebhooks()` extracts all webhooks from workflow

#### Dependencies
- Depends on: F-103, F-119
- External: - TypeORM decorators: `@Entity()`, `@Column()`, `@PrimaryColumn()`, `@Index()`
- n8n-workflow: `IWebhookData`, `Workflow`, `INode`

#### Porting Notes
- Webhook path normalization is critical: trailing slashes removed, leading slashes managed. Dynamic paths require UUID at position 0
- The `staticSegments` algorithm filters colons; used for matching incoming requests against registered webhooks
- Cache invalidation must be coordinated with DB upsert — cache set happens in `storeWebhook()` line 130

---

### F-023: Live Webhook Execution (Production URL)
**Category:** Trigger
**Complexity:** High

#### What it does
When a workflow is active, incoming HTTP requests to the production webhook URL (e.g., `https://n8n.example.com/webhook/uuid-path`) are routed to `LiveWebhooks.executeWebhook()`. The system finds the matching webhook in the DB, loads the workflow, and executes it starting from the webhook trigger node.

#### Specification
- Requests matched via `WebhookService.findWebhook()` (tries static first, then dynamic matching)
- For dynamic paths, incoming path segments are parsed and stored in `request.params` at lines 85–95
- Only active/published workflows execute (must have `activeVersion`)
- Workflow loaded from DB with `activeVersion` nodes/connections to prevent draft pollution
- Execution mode set to `'webhook'` at line 160
- Static data saved after execution at line 177
- Webhooks are cached for performance (`findCached()` attempts cache lookup before DB query)

#### Implementation
**Entry point:** `packages/cli/src/webhooks/webhook.service.ts:46-75` — `findCached()` with cache-then-DB fallback`
**Key files:**
- `packages/cli/src/webhooks/webhook.service.ts:46-75` — `findCached()` with cache-then-DB fallback
- `packages/cli/src/webhooks/webhook.service.ts:80-122` — Static and dynamic webhook lookup logic
- `packages/cli/src/webhooks/live-webhooks.ts:44-66` — `findAccessControlOptions()` for CORS

#### Dependencies
- Depends on: F-022, F-005
- External: - Redis/cache service for webhook caching
- Express.js Request/Response for HTTP handling
- n8n-workflow `Workflow` class

#### Porting Notes
- Dynamic webhook matching uses path segment count (`pathLength`) as a filter before full matching to avoid O(n) DB queries
- Cache hits bypass DB entirely — stale cache causes 404s for deleted workflows
- `activeVersion` is mandatory; workflows without it throw `NotFoundError` at line 110
- CORS handling supports multi-origin via comma-separated list in `allowedOrigins` at lines 203–216

---

### F-024: Test Webhook Execution (Manual Testing URL)
**Category:** Trigger
**Complexity:** High

#### What it does
During manual workflow execution via the editor UI, the system temporarily registers webhooks at a test URL (e.g., `https://n8n.example.com/webhook-test/uuid-path`) to allow testing webhooks without publishing the workflow. Test webhooks timeout after a configured interval and are automatically cleaned up.

#### Specification
- Registered via `TestWebhooks.needsWebhook()` which checks if workflow has webhook nodes:1
- Test webhooks stored in cache (not DB) via `TestWebhookRegistrationsService` with key format `{method}|{path}` or `{method}|{webhookId}|{pathLength}`
- TTL set on cache key: `TEST_WEBHOOK_TIMEOUT + TEST_WEBHOOK_TIMEOUT_BUFFER` (timeout + buffer for multi-main crash recovery) at line 71 in registrations service
- Timeout triggered at lines 338, 403 in test-webhooks.ts — calls `cancelWebhook()` to deactivate
- Single webhook triggers (Telegram, Slack, etc.) throw error if already active to prevent conflicts
- Test webhook registration must happen BEFORE node's `createWebhookIfNotExists()` call to catch immediate confirmations
- In multi-main setups, handler process commands creator process to clear webhooks via `clear-test-webhooks` pubsub event

#### Implementation
**Entry point:** `packages/cli/src/webhooks/test-webhook-registrations.service.ts:38-131` — Registration cache storage with TTL`
**Key files:**
- `packages/cli/src/webhooks/test-webhook-registrations.service.ts:38-131` — Registration cache storage with TTL
- `packages/cli/src/webhooks/test-webhooks.ts:40-44` — `SINGLE_WEBHOOK_TRIGGERS` blocklist
- `packages/cli/src/webhooks/test-webhooks.ts:186-203` — `handleClearTestWebhooks()` multi-main coordination

#### Dependencies
- Depends on: F-022, F-005
- External: - Cache service with `setHash()`, `getHashValue()`, `expire()` operations
- n8n-core `InstanceSettings` to detect multi-main mode
- Publisher service for pubsub commands in multi-main setups

#### Porting Notes
- Test webhooks live only in cache, not DB — no persistence across instance restarts
- Registration happens in two phases: BEFORE third-party webhook creation (line 395), THEN AFTER (line 401) with updated static data
- Chat trigger nodes allow sessionId-based paths (line 346–351) for dynamic routing
- Push notifications inform editor UI of test webhook execution and cancellation at lines 159–162, 436
- Multi-main setup requires pubsub command to avoid orphaned test webhooks on creator process failure

---

### F-025: Webhook Path Routing (Static vs. Dynamic)
**Category:** Trigger
**Complexity:** Medium

#### What it does
The system matches incoming webhook requests to registered workflows by method and path. Static paths match exactly; dynamic paths with parameters like `:id` match multiple requests and extract parameter values for the workflow.

#### Specification
- Static webhooks: direct method + path lookup in `webhook_entity` table (indexed, fast)
- Dynamic webhooks: first segment is always `webhookId` (UUID), remaining path checked for static segments
- Dynamic matching algorithm: extract path segments, compare against registered webhooks with same `method`, `webhookId`, and matching `pathLength`
- Select webhook with most static segments matched (greedy match at lines 101–119 in webhook.service.ts)
- Path parameter extraction at `live-webhooks.ts:85–95` and `test-webhooks.ts:96–100` — segments starting with `:` are captured
- Path normalization: trailing slashes removed before lookup; leading slashes stripped from paths

#### Implementation
**Entry point:** `packages/cli/src/webhooks/webhook.service.ts:124-126` — `findWebhook()` dispatcher to cached lookup`
**Key files:**
- `packages/cli/src/webhooks/webhook.service.ts:124-126` — `findWebhook()` dispatcher to cached lookup
- `packages/cli/src/webhooks/webhook.service.ts:171-180` — `isDynamicPath()` detection

#### Dependencies
- Depends on: F-022
- External: - Set data structure for O(1) path element lookup at line 99 in webhook.service.ts

#### Porting Notes
- Dynamic webhook matching is O(n) in worst case per incoming request (n = number of dynamic webhooks with same prefix)
- `pathLength` index critically reduces candidates before full matching
- Greedy matching (most static segments wins) can cause collisions if paths overlap; careful node configuration needed
- Cache key includes both static and dynamic path components to avoid mismatches

---

### F-026: Cron-Based Scheduled Trigger Activation
**Category:** Trigger
**Complexity:** Medium

#### What it does
Workflows with Schedule Trigger nodes are activated by registering CronJob objects that execute at specified intervals. The system supports cron expressions with timezone awareness and automatically executes workflows on schedule.

#### Specification
- Polling nodes are identified via `workflow.getPollNodes()` (any node with `.poll` function)
- Poll interval configured via node parameter `pollTimes.item[]` array of TriggerTime objects
- Each interval converted to cron expression via `toCronExpression()`
- CronJob registered for each expression with `ScheduledTaskManager.registerCron()` at line 175 in active-workflows.ts
- Cron execution only runs on leader instance (`if (!this.instanceSettings.isLeader) return` at line 78 in scheduled-task-manager.ts)
- First poll execution happens immediately during activation to validate config (line 161 in active-workflows.ts)
- Cron interval minimum validation: first field (seconds) cannot be `*` (must be at least 1 minute interval) at lines 164–166 in active-workflows.ts

#### Implementation
**Entry point:** `packages/core/src/execution-engine/active-workflows.ts:108-135` — Loop over poll nodes and activate each`
**Key files:**
- `packages/core/src/execution-engine/active-workflows.ts:108-135` — Loop over poll nodes and activate each
- `packages/core/src/execution-engine/scheduled-task-manager.ts:139-161` — `toCronKey()` creates deterministic key to prevent duplicates

#### Dependencies
- Depends on: F-001, F-005
- External: - `cron` npm package `CronJob` class
- n8n-workflow `toCronExpression()` utility
- Logger for debug output of active crons

#### Porting Notes
- Cron jobs store references in memory only — not persisted to DB
- Leader-only execution prevents duplicate crons in multi-main setups
- Cron key generation must be deterministic (sorted keys at line 153–160 in scheduled-task-manager.ts) to prevent registration of duplicates
- TTL on cron job creation: uses absolute timestamp, never expires until `deregisterCrons()` called
- Interval validation at runtime (line 164–166) — allows pre-validation in test mode before full activation

---

### F-027: Polling Trigger Execution
**Category:** Trigger
**Complexity:** Medium

#### What it does
When a polling trigger fires (on cron schedule), the system executes the poll function defined on the node, checks if new data exists, and if so, emits it to trigger workflow execution with error handling and retries.

#### Specification
- Poll function execution encapsulated in `createPollExecuteFn()` at line 241–287 in active-workflows.ts
- Execution wrapped in tracing span for observability
- `pollFunctions.__emit()` called with poll response data to start workflow execution at line 270
- On poll error: `pollFunctions.__emitError()` called with error object at line 282
- First poll execution is synchronous (test mode), failures throw immediately to detect config issues
- Subsequent polls (after cron tick) are async, errors passed to `__emitError()` instead of throwing
- Poll response `null` is valid (no data available, skip execution)

#### Implementation
**Entry point:** `packages/core/src/execution-engine/active-workflows.ts:108-135` — Integration with `activatePolling()`
**Key files:**
- `packages/core/src/execution-engine/active-workflows.ts:108-135` — Integration with `activatePolling()`

#### Dependencies
- Depends on: F-001, F-005
- External: - Tracing service for observability (SpanStatus.ok/error)
- Logger for workflow/node identification

#### Porting Notes
- Null response from poll means "no new data" — no workflow execution triggered
- Error handling diverges on first execution (test, throw) vs. subsequent (production, emit error)
- Poll function must return `INodeExecutionData[][]` (array of arrays) for proper workflow data structure

---

### F-028: Trigger Node Activation and emit/emitError Callbacks
**Category:** Trigger
**Complexity:** High

#### What it does
Non-polling trigger nodes (e.g., webhooks, Stripe webhooks, Telegram webhooks) are initialized by calling their `.trigger()` function. This function returns a `ITriggerResponse` with an optional `closeFunction` and sets up callbacks (`emit`, `emitError`) that the trigger uses to signal the workflow to execute.

#### Specification
- Triggers activated via `TriggersAndPollers.runTrigger()` at line 26–90
- Trigger function called with `TriggerContext` object providing emit/emitError callbacks
- Manual mode adds special `manualTriggerResponse` promise that resolves on first emit at lines 51–84
- Emit callback signature: `(data: INodeExecutionData[][], responsePromise?, donePromise?) => void`
- EmitError callback signature: `(error: ExecutionError) => void`
- SaveFailedExecution callback for recording errors without deactivating workflow
- Trigger response stored in `activeWorkflows[workflowId].triggerResponses[]` at line 106 in active-workflows.ts

#### Implementation
**Entry point:** `packages/cli/src/active-workflow-manager.ts:348-385` — Trigger emit callback implementation, saves static data and starts workflow`
**Key files:**
- `packages/cli/src/active-workflow-manager.ts:348-385` — Trigger emit callback implementation, saves static data and starts workflow
- `packages/cli/src/active-workflow-manager.ts:386-410` — Trigger emitError callback, deactivates workflow on error with retry queue

#### Dependencies
- Depends on: F-001, F-005
- External: - n8n-core `TriggerContext` class
- Execution services (WorkflowExecutionService, ExecutionService)
- Static data persistence service

#### Porting Notes
- `emit()` is async (starts workflow in background) — does not wait for execution completion
- `manualTriggerResponse` is only set in manual mode (line 51), not in production triggers
- Trigger closeFunction may be undefined — must guard with `if (response.closeFunction)` before calling
- SaveFailedExecution is separate from emitError — used for recording transient errors, not deactivation
- Event logging happens after execution starts (line 367) to track workflow-executed events

---

### F-029: Trigger Deactivation and closeFunction Cleanup
**Category:** Trigger
**Complexity:** Medium

#### What it does
When a workflow is removed/deactivated, all active triggers for that workflow must be properly shut down by calling their `closeFunction`. The system orchestrates cleanup of both in-memory trigger state and any external resources (webhooks, subscriptions, etc.) managed by the trigger node.

#### Specification
- Deactivation via `ActiveWorkflows.remove()` at line 182–198
- All trigger responses dereferenced and `closeFunction()` awaited for each at lines 190–193
- Trigger close errors wrapped in `WorkflowDeactivationError` at line 230
- `TriggerCloseError` is caught and logged without re-throwing (graceful failure) at lines 220–225
- Crons deregistered separately via `ScheduledTaskManager.deregisterCrons()` at line 188
- Workflow removed from `activeWorkflows` map at line 195 only after all triggers closed

#### Implementation
**Entry point:** `packages/core/src/execution-engine/active-workflows.ts:200-212` — `removeAllTriggerAndPollerBasedWorkflows()` for bulk deactivation at shutdown`
**Key files:**
- `packages/core/src/execution-engine/active-workflows.ts:200-212` — `removeAllTriggerAndPollerBasedWorkflows()` for bulk deactivation at shutdown

#### Dependencies
- Depends on: F-028
- External: - Error reporter for logging close failures
- ScheduledTaskManager for cron cleanup coordination

#### Porting Notes
- `closeFunction` is optional — not all triggers have cleanup requirements (webhooks stored in DB, not in-memory)
- Errors during close are non-fatal (logger.error, not throw) to ensure workflow removal completes
- Crons must be deregistered before trigger close in case cron calls trigger emit during shutdown
- Trigger close happens synchronously in loop (line 191–193) — no parallel cleanup

---

### F-030: Multi-Instance Webhook Activation (Leadership & Pub/Sub Coordination)
**Category:** Trigger
**Complexity:** High

#### What it does
In multi-main setups, only the leader instance manages webhooks and triggers in memory. When a non-leader receives an activation request, it publishes a pubsub command to the leader, which performs the actual activation and broadcasts the result to all followers via Push notifications.

#### Specification
- Multi-main detection via `this.instanceSettings.isMultiMain` at line 604 in active-workflow-manager.ts
- Non-leader publishes `'add-webhooks-triggers-and-pollers'` command with workflowId, activeVersionId, activationMode at lines 611–614
- Command handled by leader via `@OnPubSubEvent('add-webhooks-triggers-and-pollers', {instanceType: 'main', instanceRole: 'leader'})` at lines 739–742
- Leader executes `add()` with `shouldPublish: false` to prevent re-publishing at line 749
- Push notifications sent to all clients (broadcast) for `'workflowActivated'`, `'workflowFailedToActivate'` at lines 753, 766
- Pubsub command `'display-workflow-activation'` published by leader to instruct followers to update UI at line 756
- Deactivation: non-leader publishes `'remove-triggers-and-pollers'` at lines 902–905, handled at lines 930–944

#### Implementation
**Entry point:** `packages/cli/src/active-workflow-manager.ts:556-559` — `@OnLeaderTakeover()` hook for leadership transitions`
**Key files:**
- `packages/cli/src/active-workflow-manager.ts:556-559` — `@OnLeaderTakeover()` hook for leadership transitions
- `packages/cli/src/active-workflow-manager.ts:930-944` — Deactivation handler for pubsub command

#### Dependencies
- Depends on: F-022, F-053, F-054
- External: - Publisher service for publishing commands
- Push service for broadcasting notifications to UI
- PubSubCommandMap type for command structure validation

#### Porting Notes
- `shouldPublish: false` parameter is critical to prevent infinite pubsub loops
- Leadership transitions trigger automatic reactivation of all workflows via `@OnLeaderTakeover()` at line 556
- Followers do NOT hold triggers/webhooks in memory — only leader does
- Webhook DB entries persist across instances, but in-memory state (crons, trigger responses) is leader-only
- Pubsub command handlers must guard against non-leader execution via decorator `instanceRole: 'leader'`

---

### F-031: Workflow Activation Retry Queue with Exponential Backoff
**Category:** Trigger
**Complexity:** Medium

#### What it does
When a workflow fails to activate (e.g., auth error, service unavailable), the system adds it to a retry queue that periodically attempts reactivation with exponential backoff. The timeout starts at `WORKFLOW_REACTIVATE_INITIAL_TIMEOUT` and doubles up to `WORKFLOW_REACTIVATE_MAX_TIMEOUT`.

#### Specification
- Queued activations stored in `this.queuedActivations: Record<WorkflowId, QueuedActivation>` at line 77 in active-workflow-manager.ts
- Enqueued via `addQueuedWorkflowActivation()` at line 811–863 when trigger errors occur at lines 409, 546
- Retry function calls `this.add()` again with same activation mode at line 824
- On retry failure: timeout updated to `Math.min(lastTimeout * 2, WORKFLOW_REACTIVATE_MAX_TIMEOUT)` at lines 828–829
- On retry success: workflow removed from queue via `removeQueuedWorkflowActivation()` at line 692
- Authorization errors skip retry queue (`error.message.includes('Authorization') return` at line 544)
- Queue persisted in memory only — lost on instance restart

#### Implementation
**Entry point:** `packages/cli/src/active-workflow-manager.ts:501-548` — `activateWorkflow()` error handling and retry queueing`
**Key files:**
- `packages/cli/src/active-workflow-manager.ts:501-548` — `activateWorkflow()` error handling and retry queueing

#### Dependencies
- Depends on: F-028
- External: - Node.js `setTimeout()` and `clearTimeout()` for timer management

#### Porting Notes
- Queue is instance-local (memory) — not replicated in multi-main setups
- Authorization errors treated as permanent failures (no retry) at line 544
- Activation errors from webhook creation NOT caught — workflow stays in queue indefinitely
- Queue cleanup on removal must explicitly `clearTimeout()` to prevent memory leak at line 870
- Initial timeout must be long enough to avoid overwhelming external services on transient failures

---

### F-032: Webhook Node Setup Methods (checkExists, create, delete)
**Category:** Trigger
**Complexity:** Medium

#### What it does
Webhook nodes can define setup methods that are called when webhooks are being registered/deregistered with external services. These methods allow nodes to create webhook subscriptions on third-party platforms (e.g., GitHub, Stripe) and clean them up when workflows are deactivated.

#### Specification
- Setup methods defined on node type via `nodeType.webhookMethods[webhookName][methodName]`
- Method names: `'checkExists'`, `'create'`, `'delete'` at line 408 in webhook.service.ts
- Called via `runWebhookMethod()` at lines 385–404
- `HookContext` passed to methods with workflow, node, webhookData, mode, activation info
- Methods are optional — missing methods return undefined (no-op)
- Called during workflow activation (create/checkExists) and deactivation (delete) at lines 189, 279
- `checkExists()` called first; if returns true, `create()` skipped (idempotent)
- All errors (including QueryFailedError for duplicate entries) caught and handled appropriately

#### Implementation
**Entry point:** `packages/cli/src/active-workflow-manager.ts:186-224` — Integration with webhook registration, error handling for duplicates`
**Key files:**
- `packages/cli/src/active-workflow-manager.ts:186-224` — Integration with webhook registration, error handling for duplicates

#### Dependencies
- Depends on: F-022
- External: - n8n-core `HookContext` class
- NodeTypes service for retrieving node type definitions

#### Porting Notes
- Methods are async and must await completion
- Missing method implementations silently succeed (return undefined)
- Activation mode passed to methods to distinguish test vs. production activation
- Error handling must distinguish between "webhook already exists" (duplicate, safe) vs. other errors
- `create()` called even if `checkExists()` returns false — allows repair of orphaned subscriptions

---

### F-033: Manual Trigger Execution (UI Test Button)
**Category:** Trigger
**Complexity:** Medium

#### What it does
Users can execute workflows from the editor UI via the "Test" button. Manual executions can start from a manual trigger node, an arbitrary node with pinned data, or a webhook in test mode. The system allows specifying which trigger to start from and provides webhook test registration.

#### Specification
- Manual execution triggered via REST API endpoint, delegates to `TestWebhooks.needsWebhook()`
- Manual trigger nodes (`n8n-nodes-base.manualTrigger`, `@n8n/n8n-nodes-langchain.manualChatTrigger`) identified as starting points at line 7 in constants.ts
- Webhook-based workflows require test webhook registration if no data provided for trigger
- Manual mode execution mode set at line 133 in test-webhooks.ts
- `ManualExecutionService.runManually()` handles the execution flow at line 49–193
- Can specify `triggerToStartFrom` to run from a specific trigger node at line 33–34 in manual-execution.service.ts
- Execution wrapped with `WorkflowExecute` for execution engine integration

#### Implementation
**Entry point:** `packages/cli/src/manual-execution.service.ts:49-193` — `runManually()` dispatcher to different execution paths`
**Key files:**
- `packages/cli/src/manual-execution.service.ts:49-193` — `runManually()` dispatcher to different execution paths

#### Dependencies
- Depends on: F-001, F-005
- External: - n8n-core `WorkflowExecute` class
- Manual trigger node types (standard nodes-base)

#### Porting Notes
- Manual trigger with data skips webhook listening entirely (test webhooks not needed)
- Manual trigger without data requires test webhook (via `needsWebhook()`)
- Partial execution mode uses `runPartialWorkflow2()` if `runData` provided
- Full execution mode uses `run()` if no pinned data or partial state
- TriggerToStartFrom data is injected directly into `runData` without webhook processing

---

### F-034: Webhook Timeout and Cancellation for Manual Testing
**Category:** Trigger
**Complexity:** Medium

#### What it does
Test webhooks are registered with a timeout. If no webhook request arrives within the timeout period, the test webhook is automatically cancelled and the test execution fails with a timeout error. Multi-main setups coordinate cleanup across instances.

#### Specification
- Timeout duration set to `TEST_WEBHOOK_TIMEOUT` constant from @/constants at line 324 in test-webhooks.ts
- Timeout created and stored in `this.timeouts[key]` at line 403
- `cancelWebhook()` called on timeout at line 338
- Cancellation deactivates all test webhooks for the workflow via `deactivateWebhooks()` at line 444
- In multi-main setups, TTL set on cache entry via `cacheService.expire()` at line 73 in registrations service
- Multi-main: handler process publishes `'clear-test-webhooks'` command when webhook received from different instance at lines 173–176
- Creator process clears timeout when receiving pubsub command at line 198
- Editor UI notified via push: `'testWebhookReceived'` at line 159, `'testWebhookDeleted'` at line 436

#### Implementation
**Entry point:** `packages/cli/src/webhooks/test-webhooks.ts:186-203` — `handleClearTestWebhooks()` pubsub handler for multi-main coordination`
**Key files:**
- `packages/cli/src/webhooks/test-webhooks.ts:186-203` — `handleClearTestWebhooks()` pubsub handler for multi-main coordination
- `packages/cli/src/webhooks/test-webhook-registrations.service.ts:60-74` — TTL cache expiration for multi-main crash recovery

#### Dependencies
- Depends on: F-024
- External: - Node.js `setTimeout()` and `clearTimeout()`
- Cache service for TTL management in multi-main

#### Porting Notes
- Timeout is per-webhook-path, not per-workflow — multiple test webhooks for same workflow have independent timeouts
- Buffer added to TTL to ensure crash recovery window exceeds normal timeout
- Push notifications include workflow ID for UI to identify which workflow timed out
- Deactivation (webhook.service.deleteWebhook) must happen even on timeout to clean up third-party subscriptions

---

### F-035: Waiting Webhooks for Resume-on-Webhook (Wait Node & Form)
**Category:** Trigger
**Complexity:** High

#### What it does
Wait nodes and Form nodes can pause workflow execution and resume when a webhook is called. The system generates resumption URLs with HMAC signatures, validates signatures on incoming requests, and resumes suspended executions with new data from the webhook.

#### Specification
- Managed by `WaitingWebhooks` service (implements `IWebhookManager`)
- Execution ID used as path segment at line 135 in waiting-webhooks.ts
- Signature validation via HMAC-SHA256: URL prepared, signature generated, compared with timing-safe equality at lines 106–129
- Signature verification required if `execution.data.validateSignature` flag set at line 146
- Execution status checked: must be `'waiting'`, not `'running'` or `'finished'` at lines 161–171
- Wait node must have `restartWebhook === true` in webhookDescription to be considered valid restart webhook at line 248
- Node disabled after webhook received to prevent re-execution at line 72 in implementation
- `waitTill` field cleared to remove time-based resumption at line 216

#### Implementation
**Entry point:** `packages/cli/src/webhooks/waiting-webhooks.ts:196-280` — `getWebhookExecutionData()` for matching webhook and starting execution`
**Key files:**
- `packages/cli/src/webhooks/waiting-webhooks.ts:196-280` — `getWebhookExecutionData()` for matching webhook and starting execution
- `packages/cli/src/webhooks/waiting-webhooks.ts:483-520` — `deactivateWebhooks()` for cleanup on form submission

#### Dependencies
- Depends on: F-022, F-016
- External: - n8n-core `WAITING_TOKEN_QUERY_PARAM`, `prepareUrlForSigning()`, `generateUrlSignature()`
- Node.js `crypto.timingSafeEqual()` for secure comparison
- Execution repository for loading execution data

#### Porting Notes
- Execution must exist and be in correct state — all status checks are required for security
- Signature validation must use timing-safe comparison to prevent timing attacks
- Node disabled is critical to prevent duplicate execution on retry
- URL signing includes full path but excludes query parameters via `prepareUrlForSigning()`
- Form/Wait nodes can have optional `suffix` parameter for additional routing at line 135

---

### F-036: Webhook Response Modes (Synchronous, Last Node, On-Received)
**Category:** Trigger
**Complexity:** High

#### What it does
Workflows can respond to webhook requests with data from the workflow execution. The system supports different response modes: returning execution results, data from a specific node, or immediate responses without waiting for execution completion.

#### Specification
- Response mode auto-detected via `autoDetectResponseMode()` at lines 193–259 in webhook-helpers.ts
- Modes: `'responseNode'` (Wait/Response nodes), `'formPage'` (Form nodes), `'onReceived'` (immediate response), `'hostedChat'` (Chat Trigger POST)
- Response node path setup via `setupResponseNodePromise()` at lines 300–346
- Last node response extracted via `extractWebhookLastNodeResponse()` imported at line 72
- On-received response extracted via `extractWebhookOnReceivedResponse()` imported at line 73
- Response mode determined by workflow structure (child nodes, form presence)
- Response callback handles streaming responses (BinaryData) and JSON responses

#### Implementation
**Entry point:** `packages/cli/src/webhooks/webhook-last-node-response-extractor.ts` — Response extraction from last executed node`
**Key files:**
- `packages/cli/src/webhooks/webhook-last-node-response-extractor.ts` — Response extraction from last executed node
- `packages/cli/src/webhooks/webhook-on-received-response-extractor.ts` — Immediate response extraction

#### Dependencies
- Depends on: F-023
- External: - Webhook response extractor modules
- BinaryDataService for stream handling
- n8n-workflow constants: `FORM_TRIGGER_NODE_TYPE`, `FORM_NODE_TYPE`, `WAIT_NODE_TYPE`, `CHAT_TRIGGER_NODE_TYPE`

#### Porting Notes
- Form redirection special case: 3xx response from form nodes converted to 200 with redirectURL in body
- Response mode detection is deterministic based on workflow structure, not user config
- Chat responses different for GET vs. POST methods at lines 183–185
- Streaming responses require `BinaryDataService.getAsStream()` and proper stream piping at lines 313–315
- Response callback must be awaited (Promise-based) to ensure proper completion

---

### F-037: Webhook CORS Handling
**Category:** Trigger
**Complexity:** Low

#### What it does
The system responds to CORS preflight (OPTIONS) requests and sets appropriate CORS headers on webhook responses. CORS origin configuration is retrieved from webhook node parameters and validated against allowlist.

#### Specification
- OPTIONS request handling at line 56–58 in webhook-request-handler.ts
- CORS headers setup only if `origin` header present in incoming request at lines 49–54
- `Access-Control-Allow-Methods` header includes OPTIONS + found methods at line 190
- `Access-Control-Allow-Origin` set based on allowedOrigins from node config
- Multi-origin support: comma-separated list of allowed origins at line 205
- Single origin case (one allowed origin): direct header set at line 209
- Multiple origins case: check if request origin in list, use request origin if match, else default to first at lines 212–215
- `Access-Control-Max-Age` set to 300 seconds (5 min) at line 222
- `Access-Control-Allow-Headers` set from request header if present at lines 223–226

#### Implementation
**Entry point:** `packages/cli/src/webhooks/webhook-request-handler.ts:38-89` — Overall request handling with CORS setup`
**Key files:**
- `packages/cli/src/webhooks/webhook-request-handler.ts:38-89` — Overall request handling with CORS setup
- `packages/cli/src/webhooks/live-webhooks.ts:44-66` — `findAccessControlOptions()` for Live webhooks
- `packages/cli/src/webhooks/test-webhooks.ts:245-268` — `findAccessControlOptions()` for Test webhooks

#### Dependencies
- Depends on: F-023
- External: - Express.js Request/Response header management

#### Porting Notes
- `'*'` as allowedOrigins matches any origin without explicit header
- Single origin case more efficient (direct header) vs. multi-origin (checking list)
- Access-Control-Allow-Headers copied directly from request — no validation
- CORS max age allows browser to cache preflight for 5 minutes, reducing requests
- Content-Security-Policy header also set for sandboxing (separate feature) at line 142

---

### F-038: Trigger Count Tracking
**Category:** Trigger
**Complexity:** Low

#### What it does
The system counts triggers in active workflows (excluding manual trigger and certain internal triggers) and stores this count in the database. This is used for analytics and tracking workflow complexity.

#### Specification
- Trigger count calculated at line 696–803 in active-workflow-manager.ts via `countTriggers()`
- Counts: trigger nodes + poll nodes + unique webhooks
- Trigger nodes filtered: must have `.trigger` property, exclude `'manualTrigger'`, exclude nodes in `TRIGGER_COUNT_EXCLUDED_NODES`
- Excluded nodes: `EXECUTE_WORKFLOW_TRIGGER_NODE_TYPE`, `ERROR_TRIGGER_NODE_TYPE`
- Poll nodes identified via `workflow.getPollNodes()`
- Webhooks deduplicated by node name (some nodes have multiple webhooks)
- Count persisted via `workflowRepository.updateWorkflowTriggerCount()` at line 697

#### Implementation
**Entry point:** `packages/cli/src/constants.ts:8-11` — `STARTING_NODES`, `TRIGGER_COUNT_EXCLUDED_NODES` definitions`
**Key files:**
- `packages/cli/src/constants.ts:8-11` — `STARTING_NODES`, `TRIGGER_COUNT_EXCLUDED_NODES` definitions

#### Dependencies
- Depends on: F-028, F-026, F-027
- External: - n8n-workflow node type system
- WorkflowRepository for DB persistence

#### Porting Notes
- Manual trigger excluded to avoid inflating counts for common UI-only triggers
- Some triggers return multiple webhooks — deduplicated by Set of node names, not webhook count
- Error trigger and Execute Workflow trigger are internal, excluded from count
- Count updated immediately after successful activation at line 697

This comprehensive analysis covers every distinct feature of the n8n trigger system across all four trigger types (webhooks, schedules, polling, manual) and their supporting infrastructure.

---

### F-039: Execution Mode Selection (Simple vs Queue)
**Category:** Queue
**Complexity:** Medium

#### What it does
n8n supports two execution modes: 'simple' (in-process execution on the main instance) and 'queue' (distributed execution via Bull/Redis with dedicated workers). The mode is configured globally and determines whether jobs are queued or executed immediately.

#### Specification
- Controlled via `N8N_EXECUTIONS_MODE` env var or config (values: 'simple' or 'queue')
- In 'simple' mode, executions run in-process on the main server
- In 'queue' mode, jobs are enqueued and workers consume them
- Worker command forces mode to 'queue' regardless of config: `if (this.globalConfig.executions.mode !== 'queue') { this.globalConfig.executions.mode = 'queue'; }`
- ConcurrencyControlService is disabled in queue mode: `if (...this.globalConfig.executions.mode === 'queue') { this.isEnabled = false; }`
- Manual executions can be offloaded to workers via `OFFLOAD_MANUAL_EXECUTIONS_TO_WORKERS` env var

#### Implementation
**Entry point:** `packages/cli/src/commands/worker.ts:67-69 — Worker constructor`
**Key files:**
- packages/cli/src/commands/worker.ts:67-69 — forces queue mode
- packages/cli/src/concurrency/concurrency-control.service.ts:64-68 — disables concurrency in queue mode
- packages/cli/src/workflow-runner.ts:95,172-175 — determines whether to enqueue

#### Dependencies
- Depends on: F-040
- External: None specific to mode selection

#### Porting Notes
Mode selection is baked into startup; changing it requires process restart. The worker command cannot run in simple mode.

---

### F-040: Bull Queue Setup and Configuration
**Category:** Queue
**Complexity:** High

#### What it does
Creates and initializes a Bull queue for job management, configuring it with Redis prefix, settings, and client factory.

#### Specification
- Queue name is 'jobs' (constant QUEUE_NAME)
- Queue prefix derived from config: `globalConfig.queue.bull.prefix`
- Creates Bull queue with: `new BullQueue(QUEUE_NAME, { prefix, settings: {...globalConfig.queue.bull.settings, maxStalledCount: 0}, createClient: (type) => service.createClient(...) })`
- Bull settings include `maxStalledCount: 0` to prevent stalled job handling
- Uses RedisClientService to create Redis clients with type labels like `${type}(bull)` and `${type}(n8n)`
- MCP session store integrated with Redis: 86-day TTL, keys prefixed with `${redis.prefix}:mcp-session:${sessionId}`
- Queue metrics can be optionally enabled

#### Implementation
**Entry point:** `packages/cli/src/scaling/scaling.service.ts:60-108 — setupQueue method`
**Key files:**
- packages/cli/src/scaling/scaling.service.ts:60-108 — queue initialization logic
- packages/cli/src/scaling/constants.ts:3,5 — QUEUE_NAME and JOB_TYPE_NAME
- packages/@n8n/config/src/configs — contains queue.bull.* settings

#### Dependencies
- Depends on: F-053
- External: bull (npm package), ioredis, @n8n/n8n-nodes-langchain/mcp/core

#### Porting Notes
Queue is a singleton lazily initialized on first access. The `maxStalledCount: 0` disables Bull's built-in stalled job recovery. MCP server session store uses same Redis client.

---

### F-041: Job Data Serialization and Structure
**Category:** Queue
**Complexity:** Medium

#### What it does
Defines the data format for jobs enqueued to Bull, including workflow context and MCP-specific metadata for AI tool execution.

#### Specification
- JobData contains: `{ workflowId: string, executionId: string, loadStaticData: boolean, pushRef?: string, streamingEnabled?: boolean, restartExecutionId?: string, isMcpExecution?: boolean, mcpType?: 'service' | 'trigger', mcpSessionId?: string, mcpMessageId?: string, mcpToolCall?: {...} }`
- MCP tool call metadata: `{ toolName: string, arguments: Record<string, unknown>, sourceNodeName?: string }`
- `loadStaticData` flag determines whether worker must fetch fresh static data from DB
- `isMcpExecution` marks jobs triggered by MCP tool calls (for LangChain integration)
- `pushRef` enables real-time UI updates via WebSocket/Server-Sent Events
- `streamingEnabled` flag for streaming responses

#### Implementation
**Entry point:** `packages/cli/src/scaling/scaling.types.ts:18-42 — JobData type definition`
**Key files:**
- packages/cli/src/scaling/scaling.types.ts:18-42 — JobData and MCP fields
- packages/cli/src/workflow-runner.ts:386-399 — jobData construction during enqueue
- packages/cli/src/scaling/job-processor.ts:72-79 — job data validation and extraction

#### Dependencies
- Depends on: F-040
- External: n8n-workflow types

#### Porting Notes
`loadStaticData` optimization avoids redundant DB fetches. MCP fields are conditionally populated. Job must pass validation: `isObjectLiteral(job.data) && 'executionId' in job.data && 'loadStaticData' in job.data`.

---

### F-042: Worker Startup and Job Processing Registration
**Category:** Queue
**Complexity:** High

#### What it does
Initializes a worker process that subscribes to job execution, sets up concurrency limits, and registers handlers for job lifecycle events.

#### Specification
- Worker command (`n8n worker`) forces executions.mode to 'queue'
- Concurrency limit set from `N8N_CONCURRENCY_PRODUCTION_LIMIT` env var or `--concurrency` CLI flag (default 10)
- Warning if concurrency < 5: "THIS CAN LEAD TO AN UNSTABLE ENVIRONMENT"
- Worker calls `queue.process(JOB_TYPE_NAME, concurrency, async (job: Job) => {...})`
- Job processor validates job data and executes via `jobProcessor.processJob(job)`
- Worker emits `job-dequeued` event with execution/workflow/job IDs
- Worker registers Bull event listeners: `global:progress` and `error` events
- Graceful shutdown pauses queue and waits for running jobs to complete

#### Implementation
**Entry point:** `packages/cli/src/commands/worker.ts:111-172 — Worker.setupWorker and setConcurrency methods`
**Key files:**
- packages/cli/src/commands/worker.ts:74-126 — worker init and scaling service setup
- packages/cli/src/commands/worker.ts:151-172 — concurrency configuration and queue setup
- packages/cli/src/scaling/scaling.service.ts:111-137 — queue.process registration

#### Dependencies
- Depends on: F-040
- External: None specific

#### Porting Notes
Worker cannot run without queue mode. Concurrency < 5 triggers warning but doesn't prevent startup. gracefulShutdownTimeout configurable via env var (deprecated) or config.

---

### F-043: Job Enqueue with Priority
**Category:** Queue
**Complexity:** Medium

#### What it does
Adds an execution to the Bull queue with priority ranking, enabling realtime executions to be processed before batch ones.

#### Specification
- Priority range: 1 (highest) to Number.MAX_SAFE_INTEGER (lowest)
- Realtime executions get priority 50, standard get priority 100
- Bull options: `{ priority, removeOnComplete: true, removeOnFail: true }`
- `removeOnComplete/removeOnFail` automatically clean up Redis entries after job finishes/fails
- Job ID returned for later reference (cancel, query status)
- Emits `job-enqueued` event with executionId, workflowId, hostId, jobId

#### Implementation
**Entry point:** `packages/cli/src/scaling/scaling.service.ts:226-249 — addJob method`
**Key files:**
- packages/cli/src/scaling/scaling.service.ts:226-249 — job addition logic
- packages/cli/src/workflow-runner.ts:412 — priority assignment (realtime ? 50 : 100)

#### Dependencies
- Depends on: F-040, F-041
- External: bull library priority semantics

#### Porting Notes
Jobs auto-cleanup from Redis prevents memory bloat. Priority is static at enqueue time. Realtime flag passed via WorkflowRunner.run() method.

---

### F-044: Worker Job Processing and Execution
**Category:** Queue
**Complexity:** High

#### What it does
Worker receives a job from the queue, loads execution/workflow data, executes the workflow in isolation, and reports completion/failure back to main via Bull's progress mechanism.

#### Specification
- Worker fetches execution and workflow data from database
- Skips crashed executions (returns `{ success: false }`)
- Sets execution status to 'running' in DB
- Builds Workflow object with nodes, connections, static data
- Executes via WorkflowExecute or ManualExecutionService depending on execution.data structure
- Tracks running job in JobProcessor.runningJobs map keyed by job.id
- Handles lifecycle hooks: `workflowExecuteAfter` called via hook
- Sends job result via `job.progress(msg)` to main (webhook response, chunks, MCP responses)
- Derives JobFinishedProps from workflow run result or fetches from DB
- For MCP Trigger executions, invokes tool node directly and sends tool result

#### Implementation
**Entry point:** `packages/cli/src/scaling/job-processor.ts:72-369 — processJob method`
**Key files:**
- packages/cli/src/scaling/job-processor.ts:72-369 — full job processing logic
- packages/cli/src/scaling/job-processor.ts:140-160 — lifecycle hooks setup
- packages/cli/src/scaling/job-processor.ts:451-551 — tool invocation for MCP Triggers
- packages/cli/src/scaling/scaling.service.ts:115-134 — queue.process registration

#### Dependencies
- Depends on: F-042, F-005
- External: WorkflowExecute, SupplyDataContext, n8n-workflow

#### Porting Notes
`N8N_MINIMIZE_EXECUTION_DATA_FETCHING` env var controls whether to derive result from run object (faster) or fetch from DB (safer). MCP tool invocation supports both nodes with supplyData (native langchain) and execute methods. Crashed executions auto-skipped to prevent re-processing stalled jobs.

---

### F-045: Job Concurrency Limits (Per-Worker)
**Category:** Queue
**Complexity:** Medium

#### What it does
Controls how many jobs a single worker can process simultaneously, enforced at the queue.process registration.

#### Specification
- Concurrency passed to `queue.process(JOB_TYPE_NAME, concurrency, handler)`
- Each worker can have different concurrency setting (configured via --concurrency flag or env var)
- Setting too low (< 5) triggers warning but startup succeeds
- No retry mechanism at concurrency level (failures handled separately)
- Worker pauses queue on shutdown and waits for active jobs to complete

#### Implementation
**Entry point:** `packages/cli/src/commands/worker.ts:151-172 — setConcurrency and setupWorker`
**Key files:**
- packages/cli/src/commands/worker.ts:151-172 — concurrency configuration
- packages/cli/src/scaling/scaling.service.ts:111-137 — queue.process with concurrency

#### Dependencies
- Depends on: F-042
- External: Bull queue.process concurrency parameter

#### Porting Notes
Concurrency is per-worker, not global. Multiple workers each enforce their own limit independently. No load-balancing between workers; each consumes from shared queue at its own rate.

---

### F-046: Concurrency Control in Simple Mode (Production/Evaluation)
**Category:** Queue
**Complexity:** Medium

#### What it does
In simple (non-queue) mode, enforces global execution concurrency limits separately for 'production' and 'evaluation' execution modes, queueing excess executions until capacity available.

#### Specification
- Two separate queues: 'production' (webhook/trigger/chat modes) and 'evaluation' modes
- Limits configured via `N8N_EXECUTIONS_CONCURRENCY_PRODUCTION_LIMIT` and `N8N_EXECUTIONS_CONCURRENCY_EVALUATION_LIMIT` (-1 = unlimited)
- Disabled in queue mode (workers handle concurrency individually)
- ConcurrencyQueue holds execution IDs waiting for capacity
- Enqueue throttles if capacity exhausted; dequeue releases next waiting execution
- Events emitted: `execution-throttled` and `execution-released`
- Cloud deployments report telemetry when hitting limit thresholds: [5, 10, 20, 50, 100, 200] remaining capacity

#### Implementation
**Entry point:** `packages/cli/src/concurrency/concurrency-control.service.ts:25-101 — ConcurrencyControlService constructor and init`
**Key files:**
- packages/cli/src/concurrency/concurrency-control.service.ts:46-101 — initialization and queue setup
- packages/cli/src/concurrency/concurrency-queue.ts — FIFO queue with async resolution
- packages/cli/src/active-executions.ts:68,90,99,121 — ConcurrencyCapacityReservation integration

#### Dependencies
- Depends on: none
- External: None specific

#### Porting Notes
Limit of 0 throws InvalidConcurrencyLimitError. Negative limits (< -1) reset to -1 (unlimited). Simple mode only; completely bypassed in queue mode. Telemetry threshold reporting cloud-only feature.

---

### F-047: Job Failure Handling and Retry
**Category:** Queue
**Complexity:** Medium

#### What it does
Handles job processing errors, reports them via Bull's progress mechanism, and configures automatic retry behavior via Bull settings.

#### Specification
- Worker catches errors during job processing and calls `reportJobProcessingError`
- Error logged with executionId, jobId, error message/stack
- Worker sends JobFailedMessage via `job.progress()` to main before throwing
- JobFailedMessage contains: `{ kind: 'job-failed', executionId, workerId, errorMsg, errorStack }`
- Bull retry behavior configured via settings (default `maxStalledCount: 0` disables stalled-job recovery)
- Job auto-removed from queue on failure if `removeOnFail: true`
- Crashed executions detected by queue recovery process and marked as 'crashed' status
- Bull's implicit retry mechanism prevented by `maxStalledCount: 0` to allow n8n's own recovery

#### Implementation
**Entry point:** `packages/cli/src/scaling/scaling.service.ts:139-161 — reportJobProcessingError and queue error listeners`
**Key files:**
- packages/cli/src/scaling/scaling.service.ts:139-161 — error reporting logic
- packages/cli/src/scaling/scaling.service.ts:309-333 — worker error listener
- packages/cli/src/scaling/job-processor.ts:131-133 — error catching in processJob

#### Dependencies
- Depends on: F-040, F-044
- External: Bull error events, ErrorReporter

#### Porting Notes
`removeOnFail: true` prevents manual job retry via Bull UI. Error details sent via progress message so main process receives them. `maxStalledCount: 0` prevents Bull from auto-retrying stalled jobs; n8n's queue recovery handles it instead.

---

### F-048: Graceful Shutdown and Execution Cancellation
**Category:** Queue
**Complexity:** Medium

#### What it does
Ensures clean process termination by pausing job queue, waiting for in-flight jobs to complete, and allowing cancellation of queued jobs.

#### Specification
- Worker shutdown marked with @OnShutdown(HIGHEST_SHUTDOWN_PRIORITY)
- Calls `pauseQueue(true, true)` to stop enqueuing and pickup of new jobs
- Worker waits for running jobs via polling `getRunningJobsCount()` every 500ms with status logging
- Main shutdown only pauses queue if single-main setup; multi-main leaves queue running for other mains
- Job cancellation sends `abort-job` message to worker via `job.progress({ kind: 'abort-job' })`
- Worker listener on `global:progress` receives abort and calls `jobProcessor.stopJob(jobId)`
- stopJob calls `run.cancel()` on PCancelable workflow execution
- Active execution removal via `finalizeExecution` releases concurrency capacity

#### Implementation
**Entry point:** `packages/cli/src/scaling/scaling.service.ts:163-197 — stop method (main and worker paths)`
**Key files:**
- packages/cli/src/scaling/scaling.service.ts:163-197 — lifecycle shutdown
- packages/cli/src/scaling/scaling.service.ts:309-314 — abort-job listener
- packages/cli/src/scaling/job-processor.ts:408-422 — stopJob implementation
- packages/cli/src/commands/worker.ts:52-62 — worker process stopProcess

#### Dependencies
- Depends on: F-040, F-014
- External: PCancelable for cancellable promises

#### Porting Notes
Graceful timeout configured via `N8N_GRACEFUL_SHUTDOWN_TIMEOUT` or `queue.bull.gracefulShutdownTimeout`. Worker polls every 500ms, logging every 4 ticks (2s). Multi-main setups don't pause shared queue on individual shutdown.

---

### F-049: Queue Recovery (Dangling Execution Detection)
**Category:** Queue
**Complexity:** High

#### What it does
Leader-only background process that periodically detects executions marked as 'new' or 'running' in DB but absent from the queue, marking them as 'crashed' to prevent indefinite hangs.

#### Specification
- Scheduled only on leader instance via @OnLeaderTakeover decorator
- Runs in batches (default batch size from config.executions.queueRecovery.batchSize)
- Interval configured via `config.executions.queueRecovery.interval` (minutes)
- Queries DB for in-progress executions and cross-references with queue active/waiting jobs
- Dangling executions marked as 'crashed' status via `executionRepository.markAsCrashed()`
- If batch fills entire limit, speeds up next cycle by halving wait time
- Context stored: `{ timeout, batchSize, waitMs }`
- Stops on leader stepdown via @OnLeaderStepdown decorator
- Logs info on recovery completion with dangling IDs

#### Implementation
**Entry point:** `packages/cli/src/scaling/scaling.service.ts:575-605 — scheduleQueueRecovery and stopQueueRecovery`
**Key files:**
- packages/cli/src/scaling/scaling.service.ts:570-640 — queue recovery full implementation
- packages/cli/src/scaling/scaling.service.ts:611-639 — recoverFromQueue logic

#### Dependencies
- Depends on: F-040, F-009
- External: ExecutionRepository, leader election decorators

#### Porting Notes
Dangling executions are those with status 'new'/'running' but not in Bull's active/waiting jobs. Recovery speeds up if batch was full (indicates more dangling executions exist). Only runs on leader to prevent parallel recovery processes competing.

---

### F-050: Queue Metrics Collection and Prometheus Export
**Category:** Queue
**Complexity:** Medium

#### What it does
Collects job completion/failure counts at regular intervals and emits events for Prometheus metrics export.

#### Specification
- Enabled only if `config.endpoints.metrics.includeQueueMetrics === true` and instance type is 'main'
- Main instance sets up interval timer to poll `getPendingJobCounts()` (active, waiting)
- Tracks completed and failed job counters, incremented by `global:completed` and `global:failed` listeners
- Interval tick emits `job-counts-updated` event with: `{ active, waiting, completed, failed }`
- Counters reset after each tick to track per-interval throughput
- Interval duration: `config.endpoints.metrics.queueMetricsInterval` (in seconds, converted to milliseconds)

#### Implementation
**Entry point:** `packages/cli/src/scaling/scaling.service.ts:524-564 — queue metrics methods`
**Key files:**
- packages/cli/src/scaling/scaling.service.ts:532-553 — scheduleQueueMetrics and collection
- packages/cli/src/scaling/scaling.service.ts:430-433 — metric listener registration
- packages/cli/src/events/maps/queue-metrics.event-map.ts — event mapping

#### Dependencies
- Depends on: F-040
- External: EventService for emission

#### Porting Notes
Metrics only collected on main instance. Counters are per-interval; each tick resets to 0. `global:completed` and `global:failed` listeners only registered if metrics enabled. Stopped via clearInterval on stopQueueMetrics.

---

### F-051: Job Message Routing (Progress, Response, Chunk, MCP)
**Category:** Queue
**Complexity:** High

#### What it does
Routes messages sent via Bull's progress mechanism between worker and main process for webhook responses, streaming chunks, execution completion, and MCP tool results.

#### Specification
- Messages sent via `job.progress(msg)` from worker to main
- JobMessage types: RespondToWebhookMessage, JobFinishedMessage, SendChunkMessage, McpResponseMessage, JobFailedMessage, AbortJobMessage
- Main listener on `global:progress` event intercepts and routes by message.kind
- Webhook response: `activeExecutions.resolveResponsePromise(executionId, decodedResponse)`
- Send chunk (streaming): writes to UI push channel
- Job finished: resolves response promise, optionally stores result for DB-less retrieval
- Job failed: logs error with stack trace
- MCP response: routes to McpService or McpServer depending on mcpType (service/trigger)
- Worker encodes Buffer bodies as base64 in JSON; main decodes via `decodeWebhookResponse`

#### Implementation
**Entry point:** `packages/cli/src/scaling/scaling.service.ts:338-434 — registerMainOrWebhookListeners with message routing`
**Key files:**
- packages/cli/src/scaling/scaling.service.ts:338-434 — message routing logic
- packages/cli/src/scaling/scaling.types.ts:58-121 — JobMessage type definitions
- packages/cli/src/scaling/job-processor.ts:168-205 — progress message sending from worker
- packages/cli/src/scaling/scaling.service.ts:446-492 — MCP response handling

#### Dependencies
- Depends on: F-040, F-044
- External: Bull global:progress event, ActiveExecutions push mechanism

#### Porting Notes
Progress messages are internal to Bull's pubsub; different from n8n's own PubSub system. MCP responses routed to appropriate handler based on mcpType and mcpSessionId. Buffer encoding uses BINARY_ENCODING constant (__@N8nEncodedBuffer@__).

---

### F-052: Worker Status Monitoring and Reporting
**Category:** Queue
**Complexity:** Medium

#### What it does
Allows main process to query worker status (memory, CPU, running jobs) and push it to connected UI clients.

#### Specification
- Main requests worker status via `publisher.publishCommand({ command: 'get-worker-status', payload: { requestingUserId } })`
- Worker listener on PubSub event `get-worker-status` generates status via `generateStatus()`
- Status includes: running jobs summary, memory (process + available + constraint), uptime, load average, CPU count/model, architecture, platform, hostname, network interfaces, n8n version
- Worker publishes response via `publisher.publishWorkerResponse()` to `response-to-get-worker-status` channel
- Main listener routes status to requesting user via `push.sendToUsers()` with message type `sendWorkerStatusMessage`
- Memory constraint detection for containerized deployments (via `process.constrainedMemory()`)

#### Implementation
**Entry point:** `packages/cli/src/scaling/worker-status.service.ee.ts:22-57 — request and response handling`
**Key files:**
- packages/cli/src/scaling/worker-status.service.ee.ts — full implementation
- packages/@n8n/api-types — WorkerStatus type definition

#### Dependencies
- Depends on: F-040, F-042
- External: os module, process module (Node.js), PubSub system

#### Porting Notes
EE feature (service.ee.ts suffix). Only main process initiates requests. Worker status endpoint scopes to single requesting user. Container detection via process.constrainedMemory().

---

### F-053: Redis Connection with Scaling/PubSub
**Category:** Queue
**Complexity:** Medium

#### What it does
Establishes Redis connections for Bull queue, command/response pubsub channels, and session storage (MCP), with prefixing for multi-deployment isolation.

#### Specification
- RedisClientService creates three types of clients: publisher(n8n), subscriber(n8n), and Bull clients
- Each client type labeled with descriptive string for logging/identification
- Redis prefix from config: `globalConfig.redis.prefix`
- Prefixed channel names: `${prefix}:n8n.commands`, `${prefix}:n8n.worker-response`, `${prefix}:n8n.mcp-relay`
- Bull clients created via `service.createClient({ type: `${type}(bull)` })`
- MCP session store keys: `${prefix}:mcp-session:${sessionId}`
- Retry strategy handled by RedisClientService (ECONNREFUSED errors logged, not fatal)
- Automatic reconnection on connection loss

#### Implementation
**Entry point:** `packages/cli/src/scaling/scaling.service.ts:60-75 — queue setup with Redis client creation`
**Key files:**
- packages/cli/src/scaling/scaling.service.ts:60-75 — Bull client creation
- packages/cli/src/scaling/pubsub/publisher.service.ts:35-54 — publisher client and channel names
- packages/cli/src/scaling/pubsub/subscriber.service.ts — subscriber client setup

#### Dependencies
- Depends on: none
- External: ioredis (redis client library), RedisClientService

#### Porting Notes
Multiple deployments share Redis instance but isolated by prefix. MCP session store TTL hardcoded as 86400 seconds (24 days). Connection errors on worker startup with Lua script init are fatal (process.exit(1)).

---

### F-054: Multi-Main Setup with PubSub Orchestration
**Category:** Queue
**Complexity:** High

#### What it does
In queue mode with multiple main instances, coordinates workflow activation, webhook registration, and command broadcasting across main processes via Redis pubsub.

#### Specification
- MultiMainSetup (EE) handles initialization and shutdown of pubsub infrastructure
- Publisher broadcasts commands to all mains/workers via COMMAND_PUBSUB_CHANNEL
- Subscriber receives and debounces commands, with immediate-execution exceptions
- Commands include: add-webhooks-triggers-and-pollers, remove-triggers-and-pollers, relay-execution-lifecycle-event, relay-chat-stream-event, cancel-test-run, get-worker-status
- SELF_SEND_COMMANDS (activation/deactivation) sent to sender as well
- IMMEDIATE_COMMANDS bypass debouncing: webhook/trigger/chat events, test-run cancellation
- Debouncing prevents duplicate processing of same command from multiple sources
- Leader election decorators: @OnLeaderTakeover / @OnLeaderStepdown trigger queue recovery

#### Implementation
**Entry point:** `packages/cli/src/scaling/constants.ts:8-34 — channel and command constants`
**Key files:**
- packages/cli/src/scaling/constants.ts — channel and command definitions
- packages/cli/src/scaling/pubsub/publisher.service.ts:69-96 — publishCommand with debounce flag
- packages/cli/src/scaling/multi-main-setup.ee.ts — orchestration setup
- packages/cli/src/commands/start.ts:88-95 — main shutdown pubsub cleanup

#### Dependencies
- Depends on: F-053
- External: PubSub framework (n8n internal), leader election

#### Porting Notes
EE feature. Debouncing prevents cascade of duplicate webhooks/triggers across multiple main instances. Commands flagged with debounce and selfSend attributes at publish time. MCP relay channel separate for LangChain integration.

---

### F-055: Task Runner Sandboxing
**Category:** Queue
**Complexity:** High

#### What it does
Provides sandboxed execution environment for user code (typically Python) to prevent security issues and resource exhaustion.

#### Specification
- Task runner can be Node.js-based or Python-based (separate packages)
- Initialized as dependency of main and worker processes
- Prevents arbitrary code execution in main/worker threads
- Handles code compilation, execution, timeout, and resource limits
- Returns structured output to workflow execution

#### Implementation
**Entry point:** `packages/cli/src/commands/worker.ts:45 — needsTaskRunner flag`
**Key files:**
- packages/@n8n/task-runner/src — task runner implementation
- packages/@n8n/task-runner-python/src — Python task runner

#### Dependencies
- Depends on: F-042
- External: @n8n/task-runner, @n8n/task-runner-python

#### Porting Notes
Task runner is external service/library; details beyond scope of queue system. Both main and worker processes require task runner initialization.

---

### F-056: Controller Registration via Decorators
**Category:** API
**Complexity:** Medium

#### What it does
Registers REST endpoint handlers on Express.js using TypeScript class decorators. Controllers declare HTTP routes with metadata that the ControllerRegistry processes to attach middleware and configure authentication/authorization.

#### Specification
- Controllers are decorated with `@RestController('/path')` to define base path
- Route methods decorated with `@Get`, `@Post`, `@Put`, `@Patch`, `@Delete`, `@Head`, `@Options` specifying HTTP method and path
- Method parameters decorated with `@Body`, `@Query`, `@Param(key)` for extracting/validating request data
- Metadata stored in `ControllerRegistryMetadata` service and processed at app startup
- Routes registered on prefix `/{N8N_REST_ENDPOINT}/{basePath}` unless `registerOnRootPath: true`
- All routes return Promise and response handled by `send()` wrapper with error handling

#### Implementation
**Entry point:** `packages/@n8n/decorators/src/controller/rest-controller.ts:6-16 — RestController decorator`
**Key files:**
- packages/@n8n/decorators/src/controller/rest-controller.ts:6-16 — RestController decorator definition
- packages/@n8n/decorators/src/controller/route.ts:28-56 — HTTP method and RouteOptions configuration
- packages/@n8n/decorators/src/controller/args.ts:6-23 — @Body, @Query, @Param parameter injection
- packages/@n8n/decorators/src/controller/controller-registry-metadata.ts:9-36 — metadata storage and retrieval
- packages/cli/src/controller.registry.ts:43-125 — ControllerRegistry.activate() route registration

#### Dependencies
- Depends on: none
- External: Express.js Router, @n8n/di (Service, Container), @n8n/decorators

#### Porting Notes
Decorator metadata is stored in a singleton registry during class definition, then processed once during app.start(). Routes are matched in registration order. The `args` array on RouteMetadata stores decorators for each parameter (index-based). DTO validation happens via Zod safeParse on extracted request data.

---

### F-057: Zod-based Request Validation with Auto-Extraction
**Category:** API
**Complexity:** Medium

#### What it does
Automatically validates request body and query parameters using Zod schemas attached to handler method parameter types. Invalid data returns 400 with validation error details.

#### Specification
- Handler parameters typed with Zod classes (e.g., `@Body dto: LoginDto`) automatically validated
- Zod schema extracted via TypeScript `design:paramtypes` reflection metadata
- Body and query data parsed through `DTO.safeParse(req.body|req.query)`
- On validation failure, returns 400 JSON response with error array (first error only)
- Validation happens before route handler execution
- Validation errors skip to error handler, not caught by route logic
- Supports @Query with string-to-number/boolean auto-coercion via Zod transforms

#### Implementation
**Entry point:** `packages/cli/src/controller.registry.ts:100-106 — Zod safeParse validation`
**Key files:**
- packages/cli/src/controller.registry.ts:81-110 — handler wrapper with arg extraction and validation
- packages/@n8n/decorators/src/controller/args.ts:16-23 — @Body, @Query decorators
- packages/@n8n/api-types/src/dto/pagination/pagination.dto.ts:18-35 — example Zod validators with transforms

#### Dependencies
- Depends on: F-056
- External: Zod, TypeScript reflect-metadata, @n8n/api-types

#### Porting Notes
Validation schema must be attached to parameter type via TypeScript—no separate @Validate decorator needed. The first error from safeParse is returned; later errors in validation are discarded. Zod's .transform() is used for type coercion (string to int, etc.).

---

### F-058: Licensing Enforcement Decorator
**Category:** API
**Complexity:** Low

#### What it does
Restricts endpoint access to users whose n8n instance has a valid license for a specific feature. Returns 403 if feature not licensed.

#### Specification
- Route decorated with `@Licensed('featureKey')` where featureKey is BooleanLicenseFeature type
- Middleware checks `License.isLicensed(feature)` before handler runs
- Returns 403 JSON `{ status: 'error', message: 'Plan lacks license for this feature' }` if unlicensed
- Licensing check runs AFTER rate limiting, BEFORE auth/scope checks
- No propagation of licensing state to frontend in error response

#### Implementation
**Entry point:** `packages/@n8n/decorators/src/controller/licensed.ts:7-15 — Licensed decorator`
**Key files:**
- packages/@n8n/decorators/src/controller/licensed.ts:7-15 — decorator implementation
- packages/cli/src/controller.registry.ts:211-218 — license middleware factory

#### Dependencies
- Depends on: F-056, F-095
- External: @n8n/constants (BooleanLicenseFeature), License service

#### Porting Notes
License state is not checked at route registration time—only at request time. Multiple licensed routes share the same License service instance, so license check is consistent across endpoints.

---

### F-059: Scope-based Access Control (Global vs Project Level)
**Category:** API
**Complexity:** Medium

#### What it does
Verifies user has required permission scope at global or project level. `@GlobalScope` checks globally only; `@ProjectScope` checks project-level first, then falls back to global.

#### Specification
- `@GlobalScope(scope)` enforces scope globally; fails with 403 if user lacks it
- `@ProjectScope(scope)` checks project-level scope (via req.params.projectId) first, then global as fallback
- Scope validation happens AFTER auth, AFTER license, BEFORE route handler
- Returns 403 JSON `{ status: 'error', message: 'MISSING_SCOPE' }` on failure
- Route params (projectId, workflowId, credentialId) extracted and passed to scope checker
- Scope checker is async (database queries for project membership)

#### Implementation
**Entry point:** `packages/@n8n/decorators/src/controller/scoped.ts:7-52 — Scoped/GlobalScope/ProjectScope decorators`
**Key files:**
- packages/@n8n/decorators/src/controller/scoped.ts:7-52 — decorator definitions
- packages/cli/src/controller.registry.ts:221-248 — scope middleware factory and userHasScopes call

#### Dependencies
- Depends on: F-056, F-090, F-091
- External: @n8n/permissions, @/permissions.ee/check-access

#### Porting Notes
Scope checking is async and requires database access. The globalOnly flag determines whether project-level scopes are checked first (false = project first then global, true = global only).

---

### F-060: IP-based Rate Limiting
**Category:** API
**Complexity:** Low

#### What it does
Limits request rate per client IP address. Runs before authentication. Configured per-route with optional custom limits (default: 5 requests per 5 minutes).

#### Specification
- Route option `ipRateLimit: true | RateLimiterLimits` enables IP rate limiting
- Default limits: 5 requests per 5 minutes (window 300,000 ms)
- Custom limits via `{ limit: N, windowMs: M }` object
- Middleware runs BEFORE authentication (Layer 1)
- Only enforced in production (`inProduction` check)
- Rate limit exceeded returns 429 JSON `{ message: 'Too many requests' }`
- Keying by client IP extracted by express-rate-limit library

#### Implementation
**Entry point:** `packages/cli/src/services/rate-limit.service.ts:31-39 — createIpRateLimitMiddleware`
**Key files:**
- packages/cli/src/services/rate-limit.service.ts:12-39 — IP rate limit service with defaults
- packages/cli/src/controller.registry.ts:149-151 — IP rate limit middleware insertion in middleware chain
- packages/@n8n/decorators/src/controller/route.ts:18-19 — ipRateLimit option definition

#### Dependencies
- Depends on: F-056
- External: express-rate-limit library, @n8n/constants (Time)

#### Porting Notes
Rate limiting is skipped in development/test modes. The middleware uses express-rate-limit's built-in IP detection (respects X-Forwarded-For). Only one IP-based rate limiter per route.

---

### F-061: User Keyed Rate Limiting
**Category:** API
**Complexity:** Low

#### What it does
Limits request rate per authenticated user. Runs after authentication. Useful for preventing brute-force attacks on user-scoped endpoints like password reset.

#### Specification
- Route option `keyedRateLimit: createUserKeyedRateLimiter({ limit, windowMs })` enables user-keyed limiting
- Keyed by `user:${userId}` from authenticated request
- Must be used ONLY on authenticated routes (skipAuth: false)
- Middleware runs AFTER authentication (Layer 2b)
- Only enforced in production
- Default limits: 5 requests per 5 minutes
- Rate limit exceeded returns 429 JSON `{ message: 'Too many requests' }`

#### Implementation
**Entry point:** `packages/cli/src/services/rate-limit.service.ts:65-75 — createUserKeyedRateLimitMiddleware`
**Key files:**
- packages/cli/src/services/rate-limit.service.ts:65-75 — user keyed rate limit middleware
- packages/cli/src/controller.registry.ts:179-192 — user rate limit middleware insertion (after auth)
- packages/@n8n/decorators/src/controller/rate-limit.ts:59-76 — createUserKeyedRateLimiter helper

#### Dependencies
- Depends on: F-056, F-080
- External: express-rate-limit

#### Porting Notes
User keyed rate limiting requires authentication. Attempted use on skipAuth endpoints raises assertion error at build time. The service checks for 'skip:' prefixes in identifier to allow bypassing under certain conditions.

---

### F-062: Body Field Keyed Rate Limiting
**Category:** API
**Complexity:** Low

#### What it does
Limits request rate per value of a specific request body field (e.g., email address). Runs before authentication. Used for login/signup endpoints to prevent enumeration attacks.

#### Specification
- Route option `keyedRateLimit: createBodyKeyedRateLimiter({ field: 'email', limit, windowMs })` enables body field keying
- Middleware runs BEFORE authentication (Layer 2a)
- Field value extracted from req.body and validated against DTO schema
- On validation failure or missing field, request is skipped (not rate-limited)
- Default limits: 5 requests per 5 minutes
- Rate limit exceeded returns 429 JSON `{ message: 'Too many requests' }`
- Keyed by `body:${fieldValue}`

#### Implementation
**Entry point:** `packages/cli/src/services/rate-limit.service.ts:45-63 — createBodyKeyedRateLimitMiddleware`
**Key files:**
- packages/cli/src/services/rate-limit.service.ts:45-63 — body keyed rate limit middleware
- packages/cli/src/controller.registry.ts:153-166 — body rate limit insertion (before auth)
- packages/@n8n/decorators/src/controller/rate-limit.ts:40-57 — createBodyKeyedRateLimiter helper

#### Dependencies
- Depends on: F-056
- External: express-rate-limit, Zod for field validation

#### Porting Notes
Field DTO class (with Zod schema) must be provided to rate limit middleware. Missing @Body decorator or DTO class causes assertion error. Malformed body field values result in 'skip:' identifiers, bypassing rate limit for that request.

---

### F-063: Authentication Middleware with MFA Support
**Category:** API
**Complexity:** High

#### What it does
Validates JWT auth cookie and enforces MFA if enabled/required. Sets req.user for authenticated requests or returns 401.

#### Specification
- Runs on all routes except those with `skipAuth: true`
- Reads JWT token from auth cookie (`AUTH_COOKIE_NAME`)
- Checks token against invalidAuth token blacklist (revocation support)
- Decodes JWT and resolves user entity
- If MFA enforced and user lacks MFA or didn't use MFA during auth: returns 401 or requires enrollment
- `allowSkipMFA: true` bypasses MFA requirement for specific routes
- `allowUnauthenticated: true` allows request to proceed without user but sets authInfo flags
- Sets `req.user` (User entity) and `req.authInfo` (usedMfa, mfaEnrollmentRequired)
- Preview mode (`N8N_PREVIEW_MODE`) can skip auth if `allowSkipPreviewAuth: true`

#### Implementation
**Entry point:** `packages/cli/src/auth/auth.service.ts:96-157 — createAuthMiddleware`
**Key files:**
- packages/cli/src/auth/auth.service.ts:96-157 — auth middleware with MFA enforcement
- packages/cli/src/controller.registry.ts:168-177 — auth middleware mounting in chain
- packages/@n8n/decorators/src/controller/route.ts:11-17 — auth-related route options

#### Dependencies
- Depends on: F-080, F-086
- External: jsonwebtoken library, @n8n/db (User, InvalidAuthToken), MfaService

#### Porting Notes
MFA enforcement is async (database queries). JWT token is resolved to User via `resolveJwt()` method. Token revocation checked via InvalidAuthTokenRepository before decoding. allowUnauthenticated flag allows partial auth state where MFA enrollment is required.

---

### F-064: CORS Header Application (Per-Route)
**Category:** API
**Complexity:** Low

#### What it does
Applies Access-Control-* headers to response based on route configuration. Runs inside route handler after middleware, not as a middleware itself.

#### Specification
- Route option `cors: true | Partial<CorsOptions>` enables CORS headers
- CorsOptions: { allowedOrigins, allowedMethods, allowedHeaders, allowCredentials, maxAge }
- Default origins: '*' (all), methods: all HTTP verbs, headers: Origin, X-Requested-With, Content-Type, Accept
- CORS headers applied inside route handler (line 91 of controller.registry.ts)
- Returns false if no origin header in request; does not apply CORS headers
- Preflight OPTIONS requests NOT handled by this; must be handled separately
- Allows origin if '*' in allowedOrigins OR exact match

#### Implementation
**Entry point:** `packages/cli/src/services/cors-service.ts:18-46 — applyCorsHeaders`
**Key files:**
- packages/cli/src/services/cors-service.ts:1-52 — CorsService with applyCorsHeaders method
- packages/cli/src/controller.registry.ts:88-92 — CORS header application inside route handler
- packages/@n8n/decorators/src/controller/types.ts:12-18 — CorsOptions interface

#### Dependencies
- Depends on: F-056
- External: Express Response.header()

#### Porting Notes
CORS is applied per-route, not globally. Missing origin header in request causes silent skip (returns false, no headers applied). Preflight requests (OPTIONS) are not handled by this system and must be configured separately if needed.

---

### F-065: Response Envelope Wrapping (Success and Error)
**Category:** API
**Complexity:** Low

#### What it does
Wraps all successful and error responses in consistent JSON format with automatic error handling, logging, and reporting.

#### Specification
- Success responses: `{ data: T }` wrapping by default (raw=false)
- Raw responses: direct JSON or text without data wrapper (raw=true)
- Error responses: `{ code: number, message: string, hint?: string, stacktrace?: string, meta?: object }`
- Error code extracted from ResponseError.errorCode
- HTTP status code from ResponseError.httpStatusCode
- Unique constraint errors auto-transformed to "There is already an entry with this name"
- Stacktrace included in development mode only
- All errors reported to ErrorReporter (Sentry) except ResponseErrors with code <= 404
- Exceptions caught in `send()` wrapper and formatted

#### Implementation
**Entry point:** `packages/cli/src/response-helper.ts:156-183 — send() wrapper function`
**Key files:**
- packages/cli/src/response-helper.ts:13-183 — sendSuccessResponse, sendErrorResponse, send wrapper
- packages/cli/src/errors/response-errors/abstract/response.error.ts — ResponseError base class
- packages/cli/src/controller.registry.ts:117-121 — send() wrapper applied to routes

#### Dependencies
- Depends on: F-056
- External: @n8n/backend-common (Logger), ErrorReporter, Sentry integration

#### Porting Notes
The send() wrapper is applied to all routes except those with `usesTemplates: true`. Errors are reported to ErrorReporter before being sent to client. Unique constraint errors are detected by substring matching ('unique' or 'duplicate' in lowercase message).

---

### F-066: WebSocket Push Connection Lifecycle
**Category:** API
**Complexity:** High

#### What it does
Manages WebSocket connections from frontend clients for real-time push messaging. Handles connection setup, heartbeat/ping-pong, message parsing, and graceful disconnection.

#### Specification
- Client connects via HTTP upgrade to `/{REST_ENDPOINT}/push?pushRef={id}`
- Backend sets connection.isAlive flag on each pong received
- Server pings all connected clients every 60 seconds
- Clients must respond with pong within 60 seconds or connection terminated
- Connection closed by server if pong not received before next ping attempt
- Client can send JSON application-level heartbeat messages (type: 'heartbeat')
- Heartbeat messages handled separately from user messages (validated by schema, skipped)
- On message received: emits 'message' event with { pushRef, userId, msg }
- On connection close: automatically removes pushRef from active connections

#### Implementation
**Entry point:** `packages/cli/src/push/websocket.push.ts:15-65 — WebSocketPush.add()`
**Key files:**
- packages/cli/src/push/websocket.push.ts:15-89 — WebSocketPush class with connection lifecycle
- packages/cli/src/push/abstract.push.ts:35-97 — AbstractPush base with ping/heartbeat logic
- packages/@n8n/api-types/src/push/heartbeat.ts:3-11 — heartbeatMessageSchema validation

#### Dependencies
- Depends on: F-074
- External: ws library (WebSocket), @n8n/api-types (heartbeatMessageSchema)

#### Porting Notes
Connections stored in flat dictionary keyed by pushRef string. Bidirectional (client can send messages). Heartbeat check uses isAlive flag set on pong handler. Client heartbeat messages filtered before onMessageReceived emitted. Connection close event triggers automatic cleanup.

---

### F-067: SSE Push Connection Lifecycle
**Category:** API
**Complexity:** Medium

#### What it does
Manages Server-Sent Events connections from frontend clients for one-way push messaging (backend to frontend only). Unidirectional alternative to WebSocket.

#### Specification
- Client connects via HTTP GET to `/{REST_ENDPOINT}/push?pushRef={id}`
- Response headers: Content-Type: text/event-stream, Cache-Control: no-cache, Connection: keep-alive
- Initial response: ':ok\n\n' comment to confirm connection ready
- Server pings all SSE clients every 60 seconds with ':ping\n\n' comment
- Messages sent as 'data: {JSON}\n\n' formatted text
- Socket configured: setTimeout(0), setNoDelay(true), setKeepAlive(true)
- On client disconnect (request end/close or response finish): removes pushRef
- No inbound messages from client (unidirectional SSE)

#### Implementation
**Entry point:** `packages/cli/src/push/sse.push.ts:11-32 — SSEPush.add()`
**Key files:**
- packages/cli/src/push/sse.push.ts:1-48 — SSEPush class with SSE connection setup
- packages/cli/src/push/abstract.push.ts:93-97 — ping method called by timer

#### Dependencies
- Depends on: F-074
- External: Express Request/Response, Node.js http module

#### Porting Notes
Connection consists of { req, res } pair stored together. Unidirectional—client cannot send application messages. SSE is stateless and does not use heartbeat/pong mechanism. Client disconnect is handled via req.once('end'), req.once('close'), and res.once('finish') listeners.

---

### F-068: Push Backend Selection (WebSocket vs SSE)
**Category:** API
**Complexity:** Low

#### What it does
Configures whether to use WebSocket or SSE for push messaging. Determined at startup; backend selected via environment variable.

#### Specification
- Environment variable `N8N_PUSH_BACKEND` (default: 'websocket')
- Valid values: 'websocket' | 'sse'
- WebSocket selected: bidirectional, supports client-to-server messages
- SSE selected: unidirectional backend-to-frontend only
- Backend instance created at startup in Push service constructor
- No hot switching; selection is fixed for lifetime of process
- Push service abstracts backend differences via interface

#### Implementation
**Entry point:** `packages/cli/src/push/push.config.ts:1-8 — PushConfig with backend selection`
**Key files:**
- packages/cli/src/push/push.config.ts:1-8 — @Env('N8N_PUSH_BACKEND') config
- packages/cli/src/push/index.ts:45-49 — backend instance selection

#### Dependencies
- Depends on: F-066, F-067
- External: @n8n/config (@Config, @Env decorators)

#### Porting Notes
Backend selection happens during Push service instantiation. Once chosen, cannot be changed at runtime. isBidirectional flag reflects whether WebSocket (true) or SSE (false) is active.

---

### F-069: Push Message Types and Execution Streaming
**Category:** API
**Complexity:** Medium

#### What it does
Defines typed push message schema for real-time execution progress updates and other backend-to-frontend notifications.

#### Specification
- Message structure: `{ type: string, data: object }`
- Message types (union): ExecutionPushMessage | WorkflowPushMessage | HotReloadPushMessage | WebhookPushMessage | WorkerPushMessage | CollaborationPushMessage | DebugPushMessage | BuilderCreditsPushMessage | ChatHubPushMessage
- Execution messages: executionStarted, executionWaiting, executionFinished, executionRecovered, nodeExecuteBefore, nodeExecuteAfter, nodeExecuteAfterData
- executionStarted: { executionId, mode, startedAt, workflowId, flattedRunData, ... }
- nodeExecuteAfter: separate from nodeExecuteAfterData (metadata vs full data)
- nodeExecuteAfterData payload split across multiple messages if > 5 MB (MAX_PAYLOAD_SIZE_BYTES)
- PushType extracted as discriminant type for type-safe usage
- PushPayload<T> extracts data field for specific message type T

#### Implementation
**Entry point:** `packages/@n8n/api-types/src/push/index.ts:11-24 — PushMessage and PushType union`
**Key files:**
- packages/@n8n/api-types/src/push/index.ts:1-24 — PushMessage union and helpers
- packages/@n8n/api-types/src/push/execution.ts:9-101 — ExecutionPushMessage types
- packages/cli/src/push/index.ts:229-255 — payload size enforcement

#### Dependencies
- Depends on: F-066, F-067
- External: @n8n/api-types, n8n-workflow (ExecutionStatus, WorkflowExecuteMode)

#### Porting Notes
Messages are typed but sent as JSON. Large nodeExecuteAfterData messages are truncated at 5 MB and placeholder sent instead (frontend fetches full data separately). Message type is discriminant for TypeScript type narrowing.

---

### F-070: Push Message Routing (Single User, Multiple Users, Broadcast)
**Category:** API
**Complexity:** High

#### What it does
Routes push messages to specific frontend sessions (pushRef), specific user IDs, or all connected clients.

#### Specification
- `push.send(msg, pushRef)` sends to single session; logs warning if pushRef not found
- `push.sendToUsers(msg, userIds)` sends to all sessions of listed user IDs
- `push.broadcast(msg)` sends to all connected sessions
- Each method calls backend.sendToOne/sendToUsers/sendToAll
- In multi-main scaling: relays via pubsub if message needs to reach sessions on other instances
- Messages sent as JSON string or binary (asBinary flag)
- Broadcast used for workflow auto-deactivation, other global events
- Single send used for execution lifecycle events tied to specific session

#### Implementation
**Entry point:** `packages/cli/src/push/index.ts:158-183 — broadcast, send, sendToUsers methods`
**Key files:**
- packages/cli/src/push/index.ts:158-183 — Push routing methods
- packages/cli/src/push/abstract.push.ts:99-119 — backend sendToAll/sendToOne/sendToUsers
- packages/cli/src/push/index.ts:207-255 — pubsub relay logic for scaling

#### Dependencies
- Depends on: F-069
- External: Publisher service (pubsub), @n8n/api-types (PushMessage)

#### Porting Notes
Routing layer handles both direct send and pubsub relay. shouldRelayViaPubSub checks if message needs relay (worker instance or multi-main and pushRef not local). PushMessage payload trimmed at 5 MB for pubsub relay; nodeExecuteAfterData exceeding limit is omitted entirely.

---

### F-071: Push Message Serialization and Binary Frames
**Category:** API
**Complexity:** Medium

#### What it does
Converts PushMessage objects to JSON strings and optionally sends as binary WebSocket frames instead of text frames.

#### Specification
- Messages serialized via jsonStringify with replaceCircularRefs option
- Circular references converted to string markers to prevent serialization errors
- Binary flag allows sending as WebSocket binary frame (instead of text)
- Binary frames used for large node execution data to optimize compression
- SSE always sends text format (no binary option)
- WebSocket supports both text and binary via sendToOneConnection(data, asBinary)
- Serialization happens once per message before sending to multiple recipients

#### Implementation
**Entry point:** `packages/cli/src/push/abstract.push.ts:76-91 — sendTo method with serialization`
**Key files:**
- packages/cli/src/push/abstract.push.ts:76-91 — message serialization and send
- packages/cli/src/push/websocket.push.ts:71-73 — WebSocket binary frame support
- packages/cli/src/push/sse.push.ts:38-42 — SSE text-only send

#### Dependencies
- Depends on: F-070
- External: jsonStringify from n8n-workflow

#### Porting Notes
Circular reference handling is built into jsonStringify utility. Binary frames are WebSocket-only and transparent to consuming code via asBinary parameter. SSE transport always ignores asBinary flag (text only).

---

### F-072: Push Authentication and Origin Validation
**Category:** API
**Complexity:** Medium

#### What it does
Authenticates push connection requests via JWT cookie and validates request origin in production to prevent CSRF attacks.

#### Specification
- Push endpoint at `/{REST_ENDPOINT}/push` requires authenticated request
- Query parameter `pushRef` required; missing returns 400 'query parameter missing'
- In production: Origin header validated against expected host/protocol
- Validation uses Host header, X-Forwarded-Proto, X-Forwarded-Host, and Forwarded headers
- Invalid origin closes WebSocket (code 1008) or returns 400 BadRequestError for SSE
- If disconnected due to auth/validation error, frontend cannot reconnect (no retry)
- WebSocket and SSE both use same authentication middleware

#### Implementation
**Entry point:** `packages/cli/src/push/index.ts:93-156 — setupPushHandler, handleRequest`
**Key files:**
- packages/cli/src/push/index.ts:93-156 — push endpoint setup and validation
- packages/cli/src/push/origin-validator.ts — Origin header validation logic
- packages/cli/src/auth/auth.service.ts:96-157 — auth middleware used for push

#### Dependencies
- Depends on: F-066, F-080
- External: validateOriginHeaders utility, AuthService

#### Porting Notes
Origin validation is only in production (checked with inProduction flag). pushRef must be provided by frontend and is opaque to backend—used only to route messages. Missing pushRef causes immediate connection termination, not retry opportunity.

---

### F-073: Push Server HTTP Upgrade Handler
**Category:** API
**Complexity:** Medium

#### What it does
Listens for HTTP upgrade requests on the main HTTP/HTTPS server and routes WebSocket connections to the push handler.

#### Specification
- Registered on server.on('upgrade') event before app.listen
- Only for WebSocket backend (not SSE)
- Checks if request path is `/{REST_ENDPOINT}/push`
- Calls wsServer.handleUpgrade to convert request to WebSocket
- Attaches WebSocket to request.ws for push handler
- Creates synthetic ServerResponse to handle writeHead(status > 200) → closes WebSocket
- Delegates request/response handling to Express app via app.handle()

#### Implementation
**Entry point:** `packages/cli/src/push/index.ts:69-90 — setupPushServer`
**Key files:**
- packages/cli/src/push/index.ts:69-90 — WebSocket upgrade handler setup
- packages/cli/src/server.ts — calls setupPushServer in Server.start()

#### Dependencies
- Depends on: F-066
- External: ws library (WebSocket.Server), Node.js HTTP upgrade event

#### Porting Notes
HTTP upgrade handler must be registered before app.listen. Only required for WebSocket backend. SSE uses standard HTTP GET request (no upgrade). The synthetic ServerResponse trick allows Express routing to work with WebSocket upgrade.

---

### F-074: Push Endpoint Registration
**Category:** API
**Complexity:** Low

#### What it does
Registers the push endpoint route on Express app with authentication and response flush capability.

#### Specification
- Route: POST/GET `/{REST_ENDPOINT}/push` with query parameter pushRef
- Middleware: AuthService.createAuthMiddleware (skipMFA allowed)
- Handler: Push.handleRequest() which delegates to WebSocket or SSE backend
- Response object augmented with flush() method (from compression middleware)
- No error handling—errors thrown in handler caught by global error handler
- Query parameters required: pushRef

#### Implementation
**Entry point:** `packages/cli/src/push/index.ts:92-101 — setupPushHandler`
**Key files:**
- packages/cli/src/push/index.ts:92-101 — push endpoint registration
- packages/cli/src/push/types.ts:7-22 — PushRequest and PushResponse types
- packages/cli/src/server.ts — calls setupPushHandler in configure()

#### Dependencies
- Depends on: F-056
- External: Express app.use(), AuthService

#### Porting Notes
The push endpoint is registered via app.use() middleware at startup, not via the controller registry. Response.flush() is added by compression middleware and used by SSE to force data out immediately (not buffered).

---

### F-075: Pagination with Skip/Take Pattern
**Category:** API
**Complexity:** Low

#### What it does
Provides standard pagination for list endpoints using skip/take pattern (TypeORM style). Query parameters validated and capped at maximum.

#### Specification
- Query params: skip (offset, default 0) and take (limit, default 10)
- Max items per page: 250 (MAX_ITEMS_PER_PAGE constant)
- skip: must be non-negative integer, transformed from string
- take: must be non-negative integer, transformed from string, capped at 250
- Invalid integers return 400 validation error
- Negative values return validation error
- Used for paginating workflows, credentials, users, etc.
- Frontend receives count and items array in response

#### Implementation
**Entry point:** `packages/@n8n/api-types/src/dto/pagination/pagination.dto.ts:37-42 — paginationSchema`
**Key files:**
- packages/@n8n/api-types/src/dto/pagination/pagination.dto.ts:1-72 — PaginationDto with validators
- packages/cli/src/controllers/users.controller.ts:110-148 — example usage in listUsers

#### Dependencies
- Depends on: none
- External: Zod with transforms, @n8n/constants (Time utilities)

#### Porting Notes
Skip/take pattern used throughout codebase (not offset/limit for internal API). Zod transform handles string-to-int conversion. Max is enforced via Math.min(val, MAX_ITEMS_PER_PAGE). Public API uses offset/limit pattern instead (publicApiPaginationSchema).

---

### F-076: Pagination with Offset/Limit Pattern
**Category:** API
**Complexity:** Low

#### What it does
Alternative pagination pattern for public API endpoints using offset/limit (standard HTTP terminology). Query parameters validated and capped at maximum.

#### Specification
- Query params: offset (start position, default 0) and limit (page size, default 100)
- Max items per page: 250
- offset: non-negative integer, default 0
- limit: non-negative integer, default 100, capped at 250
- Invalid integers return 400 validation error
- Used in public API and some enterprise features
- Response includes count and items array

#### Implementation
**Entry point:** `packages/@n8n/api-types/src/dto/pagination/pagination.dto.ts:68-71 — publicApiPaginationSchema`
**Key files:**
- packages/@n8n/api-types/src/dto/pagination/pagination.dto.ts:44-71 — publicApiPaginationSchema with validators

#### Dependencies
- Depends on: none
- External: Zod with transforms

#### Porting Notes
Offset/limit is separate from skip/take. Public API must use publicApiPaginationSchema, not paginationSchema. Both patterns cap at 250 items per request to prevent abuse.

---

### F-077: API Key Authentication
**Category:** API
**Complexity:** Medium

#### What it does
Allows API endpoints to be accessed with API key instead of user session cookie. Identified via route option `apiKeyAuth: true`.

#### Specification
- Route decorated with apiKeyAuth: true allows requests with API key header
- API key sent via Authorization header with Bearer scheme
- Key validated against API key table in database
- Sets req.user from associated user entity
- Falls back to cookie auth if no API key header present
- Returns 401 if API key invalid or user not found
- Used for programmatic access, automation, integrations

#### Implementation
**Entry point:** `packages/@n8n/decorators/src/controller/route.ts:23 — apiKeyAuth route option`
**Key files:**
- packages/@n8n/decorators/src/controller/route.ts:22-23 — apiKeyAuth option

#### Dependencies
- Depends on: F-117
- External: API key validation service (not fully visible in provided files)

#### Porting Notes
API key auth option is stored in RouteMetadata but actual validation middleware not shown in provided code. Likely handled in auth.service.createAuthMiddleware or separate middleware layer.

---

### F-078: Middleware Method Execution
**Category:** API
**Complexity:** Low

#### What it does
Allows controller methods to be marked as middleware and executed for all routes in the controller before route handlers.

#### Specification
- Method decorated with `@Middleware()` is registered as controller middleware
- Middleware methods executed in registration order for all routes
- Middleware receives standard (req, res, next) Express middleware signature
- Can modify request (attach data, validate) before handler
- Must call next() to continue; if not called, request blocked
- Middleware methods bound to controller instance for 'this' context
- Middleware runs after global middleware but before route handler

#### Implementation
**Entry point:** `packages/@n8n/decorators/src/controller/middleware.ts:6-11 — Middleware decorator`
**Key files:**
- packages/@n8n/decorators/src/controller/middleware.ts:6-11 — Middleware decorator
- packages/cli/src/controller.registry.ts:60-62 — middleware binding and insertion

#### Dependencies
- Depends on: F-056
- External: Express RequestHandler type

#### Porting Notes
Middleware methods are stored in controller metadata and executed for all routes of that controller. Not typically used for auth/validation (those are route options). Useful for controller-wide setup like logging or state initialization.

---

### F-079: Static Router Mounting
**Category:** API
**Complexity:** Low

#### What it does
Allows controllers to mount pre-configured Express Router instances at specific paths with route-specific middleware and auth options.

#### Specification
- Controller can export static `routers` array with StaticRouterMetadata entries
- Each entry: { path, router, ...options }
- Options include skipAuth, allowUnauthenticated, middlewares, ipRateLimit, keyedRateLimit, licenseFeature, accessScope
- Router mounted under controller base path (e.g., `/api/v1/workflows/routers/custom`)
- Middleware applied to router via expression app.use(path, ...middlewares, router)
- Used for complex sub-resources or custom routing logic
- Authorization checks applied consistently with regular routes

#### Implementation
**Entry point:** `packages/cli/src/controller.registry.ts:64-76 — static router activation`
**Key files:**
- packages/cli/src/controller.registry.ts:64-76 — static router mounting
- packages/@n8n/decorators/src/controller/types.ts:49-67 — StaticRouterMetadata type

#### Dependencies
- Depends on: F-056
- External: Express Router, @n8n/decorators

#### Porting Notes
Static routers share authorization middleware with regular routes (same buildMiddlewares logic). Routers mounted before regular routes, so regular routes take precedence if paths conflict. Router middleware options are subset of route options (excludes cors, args, usesTemplates).

---

### F-080: JWT Cookie Issuance & Validation
**Category:** Auth
**Complexity:** Medium

#### What it does
Issues HTTP-only JWT cookies to authenticated users with configurable session duration and automatic refresh. Validates cookies on each request by checking revocation status, JWT signature/expiry, user existence, and hash consistency.

#### Specification
- JWT payload contains user ID, hash (derived from email/password/MFA secret), browser ID (optional, SHA256-hashed), and MFA usage flag
- Cookie is HTTP-only, with SameSite attribute (configurable: strict/lax/none), and optional Secure flag for HTTPS
- JWT expiration configurable via `jwtSessionDurationHours` (default system-dependent)
- Automatic refresh triggered when JWT has less than configured `jwtRefreshTimeout` remaining
- Browser ID mismatch causes token rejection (session hijacking prevention) on all endpoints except whitelisted ones (push, binary-data, OAuth callbacks, etc.)
- Revoked tokens stored in `InvalidAuthToken` table with expiration date for cleanup
- MFA state tracked in payload (`usedMfa` flag); tokens without MFA rejected if MFA enforcement enabled

#### Implementation
**Entry point:** `packages/cli/src/auth/auth.service.ts:204-220 — `issueCookie()`; 96-157 — `createAuthMiddleware()``
**Key files:**
- packages/cli/src/auth/auth.service.ts:20-29 — `AuthJwtPayload` interface definition
- packages/cli/src/auth/auth.service.ts:293-322 — `validateToken()` validates signature, user existence, and hash consistency
- packages/cli/src/auth/auth.service.ts:324-342 — `resolveJwt()` validates browser ID and triggers refresh
- packages/cli/src/auth/auth.service.ts:393-399 — `createJWTHash()` derives hash from email, password, and partial MFA secret
- packages/@n8n/db/src/entities/invalid-auth-token.ts — revoked token storage entity
- packages/@n8n/config/src/configs/auth.config.ts:10-18 — cookie configuration (secure, samesite)

#### Dependencies
- Depends on: none
- External: `jsonwebtoken` (JWT signing/verification), Express `Response` object

#### Porting Notes
- Hash must be recalculated from user data on every validation (enables invalidation if password/MFA secret changes)
- Browser ID validation can be bypassed on specific endpoints; consult whitelist when adding new endpoints
- Password hash used in JWT hash guarantees token invalidation if password reset occurs

---

### F-081: Email/Password Authentication
**Category:** Auth
**Complexity:** Medium

#### What it does
Authenticates users by email and password using bcrypt password comparison. Supports both native email users and users with LDAP identities who are attempting email login after LDAP is disabled.

#### Specification
- User lookup by email (case-insensitive, stored lowercased)
- Password verified via bcrypt comparison against stored hash
- If login fails but user has LDAP identity and LDAP is disabled, emit `login-failed-due-to-ldap-disabled` event and suggest password reset
- Handler returns `undefined` on failed authentication; caller must emit appropriate failure event

#### Implementation
**Entry point:** `packages/cli/src/auth/handlers/email.auth-handler.ts:23-43 — `handleLogin()``
**Key files:**
- packages/cli/src/auth/handlers/email.auth-handler.ts:11-44 — `EmailAuthHandler` decorated with `@AuthHandler()`
- packages/cli/src/controllers/auth.controller.ts:77-91 — login controller retrieves handler and calls `handleLogin()`
- packages/cli/src/services/password.utility.ts — bcrypt wrapper for password comparison

#### Dependencies
- Depends on: F-080, F-115
- External: `@n8n/decorators` (@AuthHandler), bcrypt via PasswordUtility

#### Porting Notes
- Handler is plugged into `AuthHandlerRegistry` via decorator; other auth methods (LDAP, SAML, OIDC) follow same pattern
- LDAP disabled check prevents users locked into email-only mode from getting clear feedback

---

### F-082: Password Reset Flow
**Category:** Auth
**Complexity:** Medium

#### What it does
Issues short-lived password reset tokens (valid for 20 minutes) embedded in URLs. Validates token, verifies user hasn't changed password since token issuance, and allows password change with optional MFA re-verification.

#### Specification
- Token generation via `generatePasswordResetToken()` creates JWT signed with user ID and JWT hash with 20-minute expiry
- Reset URL contains token and `mfaEnabled` flag for frontend to conditionally request MFA code
- Token validation via `resolvePasswordResetToken()` checks signature, user existence, and JWT hash consistency
- Password change requires valid MFA code if user has MFA enabled (code validated before password update)
- New JWT cookie issued after successful password change
- If user previously had LDAP identity, emit `user-signed-up` event with `wasDisabledLdapUser: true` flag
- Password reset blocked for LDAP users if LDAP is enabled (users must contact their Identity Provider)

#### Implementation
**Entry point:** `packages/cli/src/controllers/password-reset.controller.ts:61-164 — `forgotPassword()` and `changePassword()``
**Key files:**
- packages/cli/src/auth/auth.service.ts:344-357 — `generatePasswordResetToken()` and `generatePasswordResetUrl()`
- packages/cli/src/auth/auth.service.ts:359-391 — `resolvePasswordResetToken()` validates token and user
- packages/cli/src/controllers/password-reset.controller.ts:191-239 — password change endpoint with MFA validation
- packages/cli/src/services/password.utility.ts — bcrypt hash generation

#### Dependencies
- Depends on: F-081, F-115
- External: `UserManagementMailer` for email sending, `MfaService` for code validation

#### Porting Notes
- Rate limiting applied: 3 attempts per email per minute, 20 per IP per 5 minutes (with jitter middleware 200-1000ms)
- Email absence silently returns success (information leakage prevention)
- Invalid token returns 404 (not 400) to hide token existence
- LDAP user password reset throws error if LDAP enabled and `user:resetPassword` scope or manual login setting not granted

---

### F-083: LDAP Authentication & Synchronization
**Category:** Auth
**Complexity:** High

#### What it does
Authenticates users against an LDAP/Active Directory server, creates/updates local user records with LDAP identities, and optionally synchronizes all LDAP users on a configurable schedule. Supports TLS connection security, encrypted password storage, and user attribute mapping.

#### Specification
- LDAP configuration stored encrypted in Settings table (binding password encrypted via Cipher)
- Login enabled/disabled stored separately; disabling LDAP removes all LDAP identities from users
- Configuration includes connection URL, port, security (none/tls/ssl), bind admin credentials, and user search filters
- Supports custom user attribute mapping (email, firstName, lastName, userPrincipalName)
- Sync mode can be `active` (continuous) or `inactive` (user login triggers sync)
- Sync creates `AuthIdentity` records linking LDAP DN to local User
- User deactivation supported via LDAP config flag
- Email uniqueness can be enforced (default: true)
- Cannot enable LDAP if SAML or OIDC is currently active authentication method
- Two-way sync: LDAP attributes map to user fields; user email/password changes reflected in local DB

#### Implementation
**Entry point:** `packages/cli/src/modules/ldap.ee/ldap.service.ee.ts:50-86 — `LdapService` decorated with `@AuthHandler()`; 72-86 — `init()``
**Key files:**
- packages/cli/src/modules/ldap.ee/ldap.service.ee.ts:51-176 — `LdapService` implements `IPasswordAuthHandler`
- packages/cli/src/modules/ldap.ee/ldap.service.ee.ts:88-132 — `loadConfig()`, `updateConfig()`, `setConfig()` manage LDAP settings
- packages/cli/src/modules/ldap.ee/ldap.service.ee.ts:154-175 — `setGlobalLdapConfigVariables()` and `setLdapLoginEnabled()` toggle authentication method
- packages/cli/src/modules/ldap.ee/ldap.service.ee.ts:182-226+ — LDAP client creation and TLS configuration
- packages/@n8n/constants/src/ldap-config.ts — LDAP configuration schema (connection, bind, user search, mapping, sync)

#### Dependencies
- Depends on: F-080, F-116
- External: `ldapts` library (LDAP client), `@n8n/constants:LDAP_FEATURE_NAME`, Cipher for password encryption, LicenseState

#### Porting Notes
- LDAP feature is license-gated (checked via `LicenseState`)
- Config schema validation enforces required fields; invalid config throws error
- Sync timer scheduled/rescheduled when config changes; safe for crash recovery
- Each LDAP user gets unique AuthIdentity with LDAP DN as providerId
- User disabled state tracked separately from LDAP; disabling LDAP doesn't re-enable disabled users
- Binary LDAP attributes (AD thumbnailPhoto, etc.) handled via special decoding

---

### F-084: SAML 2.0 Authentication
**Category:** Auth
**Complexity:** High

#### What it does
Authenticates users via SAML 2.0 Identity Provider, with support for metadata loading from URL or XML, attribute mapping, multiple binding options (Redirect/POST), and optional Just-In-Time provisioning. Enterprise feature gated by license.

#### Specification
- SAML preferences stored in Settings table (metadata, IdP URL, attribute mapping, binding preferences)
- Metadata can be provided inline or fetched from URL (with SSL validation bypass option)
- Supports both HTTP Redirect and POST bindings for AuthnRequest and ACS
- Attribute mapping includes email, firstName, lastName, userPrincipalName with customizable SAML claim names
- Login request generation creates temporary IdP instance for testing without saving config
- State parameter (JWT-signed with 15-minute expiry) and nonce (JWT-signed with 15-minute expiry) prevent CSRF
- Service Provider (SP) metadata auto-generated with configurable signature/assertion signing requirements
- Just-In-Time provisioning creates users on first SAML login if enabled
- SSO claim-based provisioning can set instance role and project roles if `scopesProvisionInstanceRole` enabled
- Relay state captures user's intended destination after successful login

#### Implementation
**Entry point:** `packages/cli/src/modules/sso-saml/saml.service.ee.ts:42-117 — `SamlService` initialization and metadata loading; 169-201 — `getLoginRequestUrl()``
**Key files:**
- packages/cli/src/modules/sso-saml/saml.service.ee.ts:42-117 — `SamlService` with lazy loading of samlify library
- packages/cli/src/modules/sso-saml/saml.service.ee.ts:120-162 — IdP instance creation with schema validation
- packages/cli/src/modules/sso-saml/saml.service.ee.ts:137-162 — `getIdentityProviderInstance()` caches and validates IdP
- packages/cli/src/modules/sso-saml/saml-helpers.ts — user creation/update from SAML attributes
- packages/cli/src/modules/sso-saml/saml-validator.ts — SAML metadata and response validation
- packages/@n8n/api-types — SamlPreferences DTO with mapping and config fields
- packages/@n8n/config/src/configs/sso.config.ts:4-12 — SamlConfig with loginEnabled and loginLabel

#### Dependencies
- Depends on: F-080, F-116
- External: `samlify` library (lazy-loaded), `axios` for metadata URL fetch, XML schema validators, proxy agents for network

#### Porting Notes
- SAML feature is license-gated; invalid metadata in DB triggers automatic reset to email-only and warning log
- Config corruption recovery: if initialization fails with metadata errors, SAML is disabled and email login restored
- Service Provider instance regenerated on each attribute mapping change
- Metadata URL fetch respects HTTP_PROXY and HTTPS_PROXY environment variables
- Relay state defaults to instance base URL if not provided by SP
- WantAssertionsSigned and WantMessageSigned both configurable (default: true for both)

---

### F-085: OIDC/OpenID Connect Authentication
**Category:** Auth
**Complexity:** High

#### What it does
Authenticates users via OIDC-compliant Identity Providers with dynamic discovery, state/nonce CSRF protection, and optional Just-In-Time provisioning. Supports custom ACR values and account selection prompts.

#### Specification
- OIDC configuration stored encrypted in Settings table (client ID, secret, discovery endpoint, prompt setting)
- Client secret redacted in API responses via `OIDC_CLIENT_SECRET_REDACTED_VALUE` constant
- Discovery endpoint URL validated on config update
- State parameter generated as `n8n_state:<UUID>`, JWT-signed with 15-minute expiry
- Nonce parameter generated as `n8n_nonce:<UUID>`, JWT-signed with 15-minute expiry
- State verification validates JWT signature and format, checks for `n8n_state` prefix and valid UUID format
- Prompt parameter configurable: `select_account`, `login`, `consent`, or custom space-separated values
- Authentication Context Class Reference (ACR) values sent in AuthnRequest as optional claim
- Callback URL derived from instance base URL + `/rest/sso/oidc/callback`
- Client configuration fetched from discovery endpoint on callback
- User provisioning creates local user if OIDC JWT indicates first login and JIT provisioning enabled

#### Implementation
**Entry point:** `packages/cli/src/modules/sso-oidc/oidc.service.ee.ts:58-111 — `OidcService` with state/nonce generation; 113-143 — state verification`
**Key files:**
- packages/cli/src/modules/sso-oidc/oidc.service.ee.ts:37-56 — default OIDC config and runtime config types
- packages/cli/src/modules/sso-oidc/oidc.service.ee.ts:78-103 — `init()` loads config and OpenID client library
- packages/cli/src/modules/sso-oidc/oidc.service.ee.ts:105-143 — `generateState()`, `verifyState()`, `generateNonce()`, `verifyNonce()`
- packages/cli/src/modules/sso-oidc/oidc.service.ee.ts:93-95 — `getCallbackUrl()` constructs callback URL
- packages/@n8n/config/src/configs/sso.config.ts:15-19 — OidcConfig with loginEnabled flag
- packages/@n8n/api-types — OidcConfigDto with all configuration fields

#### Dependencies
- Depends on: F-080, F-116
- External: `openid-client` library (lazy-loaded), JWT for state/nonce signing via JwtService, Cipher for secret encryption

#### Porting Notes
- OIDC feature is license-gated; config encrypted similarly to LDAP
- State/nonce both use JWT for tamper-proofing; 15-minute window for callback
- State format strictly validated: `n8n_state:<valid-uuid>`, no flexibility
- Discovery endpoint URL must be valid HTTPS (in production) or HTTP (in development)
- EnvHttpProxyAgent used for openid-client to respect HTTP_PROXY environment
- User creation delegates to ProvisioningService (handles JIT provisioning logic)

---

### F-086: MFA (Multi-Factor Authentication)
**Category:** Auth
**Complexity:** High

#### What it does
TOTP-based MFA using OTPAuth library with recovery codes. MFA enforcement is license-gated. Users can enable/disable MFA with encrypted secret storage and recovery code management.

#### Specification
- TOTP secret generated as base32-encoded random bytes, encrypted before storage in User.mfaSecret
- Recovery codes are UUIDs (10 by default), encrypted and stored in User.mfaRecoveryCodes array
- TOTP verification uses OTPAuth with default window of 2 time-steps (±30 seconds)
- MFA enforcement state cached in CacheService (key: `mfa:enforce`); loaded from Settings on init
- MFA enforcement is license-gated: `LicenseState.isMFAEnforcementLicensed()` must return true
- Enforcement applies only if user has `mfaEnabled: true`; non-enforced users can skip MFA
- Recovery codes are single-use; used code removed from array after validation
- MFA disable requires valid MFA code or valid recovery code for verification
- TOTP URI generation includes issuer name, label, and secret for QR code generation

#### Implementation
**Entry point:** `packages/cli/src/mfa/mfa.service.ts:15-26 (service definition); 27-66 — MFA enforcement control; 103-126 — validation logic`
**Key files:**
- packages/cli/src/mfa/mfa.service.ts:35-43 — `generateRecoveryCodes()` creates UUIDs
- packages/cli/src/mfa/mfa.service.ts:45-58 — `enforceMFA()` with license check, caches enforcement state
- packages/cli/src/mfa/mfa.service.ts:60-66 — `isMFAEnforced()` checks cache, falls back to DB
- packages/cli/src/mfa/mfa.service.ts:68-87 — `saveSecretAndRecoveryCodes()` encrypts and saves to DB
- packages/cli/src/mfa/mfa.service.ts:103-126 — `validateMfa()` verifies code or recovery code
- packages/cli/src/mfa/totp.service.ts:6-36 — TOTP secret generation, URI generation, and verification via OTPAuth
- packages/cli/src/mfa/constants.ts — MFA_ENFORCE_SETTING key for Settings table

#### Dependencies
- Depends on: F-080
- External: `otpauth` library (OTPAuth class), Cipher for encryption, CacheService for enforcement state caching

#### Porting Notes
- MFA enforcement is advisory: non-MFA users bypass enforcement check even if enabled
- Recovery codes are UUIDs (36 characters), not short alphanumeric codes
- TOTP window of 2 allows for clock skew up to ±60 seconds
- Encryption happens in-memory before DB save; decryption happens on retrieval
- Recovery codes never returned in API responses; only count or "masked" view
- MFA disable via recovery code triggers removal of that code from array (semi-destructive)

---

### F-087: User Signup via Invite Links
**Category:** Auth
**Complexity:** Medium

#### What it does
Validates invite tokens (JWT-signed with 90-day expiry), allows invitees to set password and complete account setup. Enforces user quota and SSO restrictions.

#### Specification
- Invite link token contains `inviterId` and `inviteeId`, signed for 90 days
- Token validation: `GET /auth/resolve-signup-token?token=<jwt>&inviteeId=<id>`
- Invitation blocked if:
- SSO (SAML/OIDC) is current authentication method (invite links not supported)
- User quota exceeded and invitee is not owner
- Inviter doesn't exist or is missing email/firstName (caller can't be identified)
- Invitee already has password (already setup)
- Invitee not found or is not in pending state
- Successful validation returns inviter name (firstName, lastName) for display
- Token click event `user-invite-email-click` emitted
- Invite link generation: `POST /users/:id/invite-link` creates JWT with inviter and invitee IDs

#### Implementation
**Entry point:** `packages/cli/src/controllers/auth.controller.ts:199-262 — `resolveSignupToken()` endpoint`
**Key files:**
- packages/cli/src/controllers/auth.controller.ts:199-262 — signup token resolution with validations
- packages/cli/src/controllers/users.controller.ts:172-198 — invite link generation endpoint
- packages/cli/src/auth/auth.service.ts:344-357 — JWT token generation for invites (same mechanism as password reset)
- packages/cli/src/services/user.service.ts:178-217 — user creation during signup

#### Dependencies
- Depends on: F-081, F-115
- External: JwtService for token signing (90-day expiry), UserService, UrlService for base URL

#### Porting Notes
- Invite links are incompatible with SSO-only instances (SAML/OIDC as primary auth)
- Inviter must have email and firstName populated (not null); missing either blocks invite
- Invitee is "pending" if password is null AND no external auth identity AND not owner
- Token valid for 90 days; no shorter expiry configurable via API
- Invite generation creates URL with base URL + `/signup?token=<jwt>`; frontend handles navigation

---

### F-088: User Invitation & Email Sending
**Category:** Auth
**Complexity:** Medium

#### What it does
Admin users invite new users by email, sending invitation emails with signup links and generating invite URLs. Integrates with user creation flow.

#### Specification
- Invite endpoint: `POST /users/:id/invite-link` generates JWT token and URL
- User must be pending (password null, no external auth identity, not owner) to be invitable
- Generated link: `<base-url>/signup?token=<jwt>`
- Invitation email sent via UserManagementMailer with inviter details
- Invite generation restricted to users with `user:generateInviteLink` scope (typically admins)
- Invite creation event emitted for tracking

#### Implementation
**Entry point:** `packages/cli/src/controllers/users.controller.ts:172-198 — `generateInviteLink()` endpoint`
**Key files:**
- packages/cli/src/controllers/users.controller.ts:172-198 — invite link generation
- packages/cli/src/services/user.service.ts:130-161 — `addInviteUrl()` helper
- packages/cli/src/auth/auth.service.ts:344-357 — JWT token signing for invites

#### Dependencies
- Depends on: F-115
- External: JwtService, UrlService, UserManagementMailer

#### Porting Notes
- Invite link is publicly shareable; JWT contains no sensitive data beyond user IDs
- Invite URL depends on frontend handling `/signup` route with `token` query param
- Rate limiting on forgot-password also applies indirectly (uses same backend infrastructure)

---

### F-089: Logout & Token Invalidation
**Category:** Auth
**Complexity:** Low

#### What it does
Logout endpoint invalidates JWT cookie and revokes the token by adding it to InvalidAuthToken table for cleanup.

#### Specification
- Logout endpoint: `POST /auth/logout`
- Calls `invalidateToken()` which extracts token from cookie and inserts into InvalidAuthToken table with expiration date
- Cookie cleared from response
- Token revocation is asynchronous logging to warn if it fails (doesn't block logout)
- Tokens checked against InvalidAuthToken table on every request validation

#### Implementation
**Entry point:** `packages/cli/src/controllers/auth.controller.ts:264-270 — `logout()` endpoint`
**Key files:**
- packages/cli/src/auth/auth.service.ts:188-202 — `invalidateToken()` with error handling
- packages/cli/src/auth/auth.service.ts:184-186 — `clearCookie()` removes cookie
- packages/cli/src/auth/auth.service.ts:106-107 — revocation check on auth middleware

#### Dependencies
- Depends on: F-080
- External: InvalidAuthTokenRepository, JwtService for token decode

#### Porting Notes
- Token revocation is optional (errors logged but don't fail logout)
- Revocation table should have TTL cleanup or periodic DELETE for expired entries
- Cookie clearing happens regardless of revocation success

---

### F-090: Role-Based Access Control (RBAC)
**Category:** Auth
**Complexity:** High

#### What it does
Global roles (Owner, Admin, Member, ChatUser) and project roles (Owner, Admin, Editor, Viewer, ChatUser) define permissions via scopes. Roles are immutable system roles stored in database with associated scope mappings.

#### Specification
- Global roles: `global:owner`, `global:admin`, `global:member`, `global:chatUser`
- Project roles: `project:owner`, `project:admin`, `project:editor`, `project:viewer`, `project:chatUser`
- Workflow/credential sharing roles: `workflow:owner`, `workflow:editor`, `credential:owner`, `credential:editor`
- Each role has list of scopes (e.g., `user:create`, `workflow:read`, `workflow:execute`)
- Roles loaded eagerly with scopes via `relations: ['role']` on User queries
- Role assignment: `global:owner` is immutable singleton; others assignable via `PATCH /users/:id/role` with `feat:advancedPermissions` license
- Custom project roles supported with schema validation (must start with `project:` prefix)
- Scopes are composable: global scopes apply to all resources, project scopes apply to resources in that project

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/role.ts:1-62 — Role entity; packages/@n8n/db/src/constants.ts:45-76 — role definitions`
**Key files:**
- packages/@n8n/db/src/entities/role.ts — Role entity with scopes via join table
- packages/@n8n/db/src/constants.ts:52-68 — GLOBAL_OWNER_ROLE, GLOBAL_ADMIN_ROLE, GLOBAL_MEMBER_ROLE constants
- packages/@n8n/permissions/src/types.ee.ts:57-65 — role type definitions (GlobalRole, ProjectRole, etc.)
- packages/@n8n/permissions/src/roles/all-roles.ts — ALL_ROLES definition with scopes per role
- packages/cli/src/controllers/users.controller.ts:341-374 — role change endpoint with license check

#### Dependencies
- Depends on: F-115
- External: @n8n/permissions package with role schemas, License for `feat:advancedPermissions`

#### Porting Notes
- Roles are immutable except custom project roles
- Role slugs follow pattern: `<namespace>:<name>` (e.g., `global:owner`, `project:editor`)
- `systemRole: true` marks built-in roles that cannot be edited
- `roleType` field differentiates global, project, workflow, credential roles
- Owner role uniquely special: no admin can change owner role, owner cannot be changed to other roles

---

### F-091: Permission Scopes & Global Scope Checks
**Category:** Auth
**Complexity:** Medium

#### What it does
Fine-grained permission system where each action requires specific scope(s). Scopes are checked via `@GlobalScope()` decorator on endpoints or via `hasGlobalScope()` utility function.

#### Specification
- Scopes follow pattern: `<resource>:<operation>` (e.g., `user:create`, `workflow:read`, `apiKey:manage`)
- Wildcard scopes: `<resource>:*` or `*` for all operations
- Scopes defined in `@n8n/permissions` package and loaded from database Role.scopes
- Endpoint-level check: `@GlobalScope('user:create')` decorator validates caller has scope
- Runtime check: `hasGlobalScope(user, 'workflow:read')` returns boolean
- Scopes defined per role; users inherit scopes from their role
- API key scopes configurable per key (license-gated feature)

#### Implementation
**Entry point:** `packages/@n8n/decorators — `@GlobalScope()` decorator; packages/@n8n/permissions/src/utilities/has-global-scope.ee.ts — `hasGlobalScope()` utility`
**Key files:**
- packages/@n8n/permissions/src/utilities/has-global-scope.ee.ts — `hasGlobalScope()` checks if user has scope
- packages/@n8n/permissions/src/scope-information.ts — scope definitions with display names
- packages/@n8n/permissions/src/roles/scopes/global-scopes.ee.ts — global scope definitions
- packages/cli/src/controllers/users.controller.ts:111 — example: `@GlobalScope('user:list')`
- packages/cli/src/controllers/users.controller.ts:221 — example: `@GlobalScope('user:delete')`

#### Dependencies
- Depends on: F-090
- External: @n8n/permissions package, @n8n/decorators for decorator support

#### Porting Notes
- Scopes are stored as plain text (not checksummed) in database
- Decorator-based checks happen in middleware before endpoint execution
- Runtime checks done inside endpoints after authentication
- Scope list is extensive; refer to ALL_SCOPES in constants for exhaustive list

---

### F-092: Credential Sharing with Role-Based Access
**Category:** Auth
**Complexity:** Medium

#### What it does
Credentials shared from one project to another with granular roles. SharedCredentials table tracks which projects have access and at what role level.

#### Specification
- Sharing roles: `credential:owner`, `credential:editor`, `credential:viewer` (viewer not supported for credentials; only owner/editor)
- Owner can manage credentials (execute, edit, delete); editor can use in workflows
- Sharing stored in SharedCredentials table (credentialsId, projectId, role)
- Shared credentials appear in recipient project's credential list if they have appropriate role
- Credential sharing tied to project membership; must have project access to see shared credentials
- Sharing scope checks via `credential:read`, `credential:write` scopes

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/shared-credentials.ts — SharedCredentials entity`
**Key files:**
- packages/@n8n/db/src/entities/shared-credentials.ts:1-24 — SharedCredentials entity with role column
- packages/@n8n/permissions/src/types.ee.ts:60 — CredentialSharingRole type
- packages/@n8n/permissions/src/roles/scopes/credential-sharing-scopes.ee.ts — credential sharing scope definitions
- packages/cli/src/executions/pre-execution-checks/credentials-permission-checker.ts — runtime credential access check

#### Dependencies
- Depends on: F-090, F-110
- External: CredentialsEntity, Project entity, permission system

#### Porting Notes
- Credentials not directly linked to users; sharing is project-to-project
- Shared credential access depends on user's project membership and role
- Credential owner (via SharedCredentials.role='credential:owner') can revoke access
- Shared credentials are read-only for non-owner roles

---

### F-093: Workflow Sharing with Role-Based Access
**Category:** Auth
**Complexity:** Medium

#### What it does
Workflows shared within a project or between projects with role-based access. SharedWorkflow table tracks workflow-project relationships and access roles.

#### Specification
- Sharing roles: `workflow:owner`, `workflow:editor`, `workflow:viewer`
- Owner: can execute, edit, delete, manage sharing
- Editor: can execute and edit
- Viewer: can execute only
- Workflow ownership is immutable; owner project cannot change (stored on SharedWorkflow with role='workflow:owner')
- Scopes include `workflow:read`, `workflow:execute`, `workflow:update`, `workflow:delete`
- Project members with appropriate scope see shared workflows in their scope
- Workflow access aggregated by project role and workflow role combinations

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/shared-workflow.ts — SharedWorkflow entity`
**Key files:**
- packages/@n8n/db/src/entities/shared-workflow.ts:1-24 — SharedWorkflow entity with role column
- packages/@n8n/permissions/src/roles/scopes/workflow-sharing-scopes.ee.ts — workflow sharing scope definitions
- packages/cli/src/workflows/workflow-sharing.service.ts — workflow sharing permission logic
- packages/cli/src/workflows/workflow-sharing.service.ts:37-71 — `getSharedWorkflowIds()` gets IDs user can access

#### Dependencies
- Depends on: F-090, F-130
- External: WorkflowSharingService, RoleService for scope-based filtering, permission system

#### Porting Notes
- Workflow owner is project (not user); users access via project membership
- Workflow sharing immutable: project that creates workflow is always owner
- Scope-based queries use RoleService to resolve which project/workflow roles allow an operation
- "Shared with me" workflows are those user's project owns but are editor/viewer in another project's perspective

---

### F-094: Project Membership & Access Control
**Category:** Auth
**Complexity:** High

#### What it does
Users assigned to projects with roles (Owner, Admin, Editor, Viewer, ChatUser). ProjectRelation table links users to projects with role-based permissions.

#### Specification
- Each user has personal project (auto-created, owner role)
- Users can be added to team projects with specific roles
- Project owner can add/remove members and assign roles
- Project role determines what user can do with project's resources
- User's project relations loaded via `relations: ['projectRelations.role', 'projectRelations.project']`
- Access inherited: if user has `project:admin` role, they admin the project

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/project-relation.ts — ProjectRelation entity`
**Key files:**
- packages/@n8n/db/src/entities/project-relation.ts — ProjectRelation (userId, projectId, roleSlug)
- packages/@n8n/db/src/entities/project.ts — Project with projectRelations collection
- packages/cli/src/services/user.service.ts:179-198 — user to public API with project relations
- packages/cli/src/services/ownership.service.ts:123-141 — personal project owner retrieval

#### Dependencies
- Depends on: F-090, F-120, F-131
- External: Role system, project management services

#### Porting Notes
- Personal project (project:owner) created automatically for each user
- Team projects require explicit membership (ProjectRelation record)
- Role slug referenced in ProjectRelation must exist in Role table
- Project transfer on user deletion requires transferring all ProjectRelations to new owner

---

### F-095: License Feature Gates
**Category:** Auth
**Complexity:** Medium

#### What it does
Enterprise features are gated behind license checks. License service exposes boolean and numeric feature checks for conditionally enabling/disabling functionality.

#### Specification
- License managed by License service with LicenseManager from `@n8n_io/license-sdk`
- Features checked via `License.isLicensed('<feature>')` or `License.isWithinQuota('<quota>')`
- Feature examples: `feat:advancedPermissions`, `feat:saml`, `feat:oidc`, `feat:ldap`, `feat:mfa`
- Quota examples: `users` quota limits active user count
- License renewal automatic if `N8N_LICENSE_AUTO_RENEWAL_ENABLED=true`
- License cert stored in Settings table (SETTINGS_LICENSE_CERT_KEY) or loaded from N8N_LICENSE_CERT env var
- License refresh triggers PubSub event `reloadLicense` for cluster-wide updates
- Offline mode for non-main instances (license not renewed)

#### Implementation
**Entry point:** `packages/cli/src/license.ts:36-129 — License service initialization and feature checking`
**Key files:**
- packages/cli/src/license.ts:206+ — `isLicensed()`, `isWithinUsersLimit()` feature/quota checks
- packages/cli/src/license.ts:53-129 — `init()` with license manager setup
- packages/cli/src/license.ts:131-144 — `loadCertStr()` loads license from DB or env
- packages/@n8n/constants/src/license-constants.ts — LICENSE_FEATURES and LICENSE_QUOTAS definitions
- packages/cli/src/mfa/mfa.service.ts:45-58 — MFA enforcement license check example

#### Dependencies
- Depends on: none
- External: `@n8n_io/license-sdk` (LicenseManager), License state (LicenseState bean)

#### Porting Notes
- License SDK handles renewal; n8n code only checks features via License service
- Feature names are strings (e.g., 'feat:advancedPermissions'); no type safety on invalid features
- Users quota checked on login: non-owners cannot login if quota exceeded
- License changes broadcast via PubSub; changes take effect on next request (no immediate refresh)
- EULA acceptance may be required before accessing some features

---

### F-096: API Key Authentication & Scope Validation
**Category:** Auth
**Complexity:** Medium

#### What it does
Public API keys allow programmatic access with granular scopes. API keys are JWT-based or legacy format, stored encrypted, and validated on each request.

#### Specification
- API key created with optional expiration date and list of scopes
- Key format: JWT signed with `aud: 'public-api'`, `iss: 'n8n'`, or legacy `n8n_api_` prefix
- JWT API keys automatically expired after `expiresAt` timestamp
- Scope validation: API key scopes must be subset of user's available scopes (role-dependent)
- API key scopes are license-gated feature: if not licensed, all available scopes for user's role used
- API keys redacted in API responses (show last 4 characters only)
- Keys stored in ApiKey table (userId FK, label unique per user, scopes JSON, audience)

#### Implementation
**Entry point:** `packages/cli/src/controllers/api-keys.controller.ts:42-68 — create API key endpoint`
**Key files:**
- packages/cli/src/controllers/api-keys.controller.ts — CRUD endpoints for API keys
- packages/cli/src/services/public-api-key.service.ts:39-55 — API key creation with JWT signing
- packages/cli/src/services/public-api-key.service.ts:127-175 — `getAuthMiddleware()` validates API key on request
- packages/@n8n/db/src/entities/api-key.ts — ApiKey entity with scopes column
- packages/@n8n/permissions/src/public-api-permissions.ee.ts — API key scope definitions

#### Dependencies
- Depends on: F-077, F-117
- External: JwtService for JWT signing, PublicApiKeyService, @n8n/permissions for scope definitions

#### Porting Notes
- API key scopes feature requires license; without it, all user-available scopes granted
- Label must be unique per user (enforced by DB constraint)
- Keys redacted by revealing last 4 chars; prefix with asterisks
- Legacy API keys (`n8n_api_` prefix) bypass JWT validation
- Scope validation prevents users from creating keys with scopes they don't have
- API key audience is always `public-api`; other audiences possible via audience parameter

---

### F-097: Instance Owner Setup
**Category:** Auth
**Complexity:** Medium

#### What it does
First-time instance setup where the first user becomes global owner with full permissions. Setup blocked after owner is established.

#### Specification
- Setup endpoint: `POST /owner/setup` (if no owner exists yet)
- Request: email, password, firstName, lastName
- Creates first User record with role='global:owner'
- Personal project created for owner
- Owner cannot be changed or deleted without special handling
- After owner setup, endpoint becomes unavailable (returns 404 or error)

#### Implementation
**Entry point:** `Varies by implementation; typically owner setup controller`
**Key files:**
- packages/cli/src/services/ownership.service.ts — ownership management
- packages/@n8n/db/src/constants.ts:52 — GLOBAL_OWNER_ROLE definition

#### Dependencies
- Depends on: F-115, F-081
- External: User service, Project service, password hashing

#### Porting Notes
- Owner setup is one-time; once completed, endpoint blocked
- Owner role is singleton; only one owner can exist
- Owner auto-added to personal project as owner
- Owner deletion requires data transfer to other user or deletion

---

### F-098: Browser ID Session Hijacking Prevention
**Category:** Auth
**Complexity:** Medium

#### What it does
Prevents session hijacking by validating a client-generated browser ID on each request. Browser ID is hashed and stored in JWT.

#### Specification
- Browser ID is unique string generated by frontend and sent in request headers
- On login, browser ID hashed (SHA256) and embedded in JWT
- On subsequent requests, browser ID in request must hash to value in JWT
- Validation skipped on whitelisted endpoints (push, binary-data, OAuth callbacks, type files, chat attachments)
- GET requests skip browser ID validation on specific endpoints
- Mismatch logs warning and rejects request with 401 Unauthorized

#### Implementation
**Entry point:** `packages/cli/src/auth/auth.service.ts:168-174 — `getBrowserId()`; 276-291 — `validateBrowserId()``
**Key files:**
- packages/cli/src/auth/auth.service.ts:20-29 — JWT includes optional browserId (hashed)
- packages/cli/src/auth/auth.service.ts:72-93 — whitelist of endpoints skipping browser ID check
- packages/cli/src/auth/auth.service.ts:276-291 — browser ID validation logic
- packages/cli/src/auth/auth.service.ts:331-334 — browser ID passed during JWT refresh

#### Dependencies
- Depends on: F-080
- External: None (SHA256 hash via crypto module)

#### Porting Notes
- Browser ID sent by frontend; backend validates matching
- Endpoint whitelist is hardcoded; adding new endpoints requires code change
- GET requests have reduced browser ID checking (only if browserId in JWT)
- Session hijacking prevention is optional per architecture; endpoints can require it

---

### F-099: Authentication Method Switching (Email ↔ LDAP ↔ SAML ↔ OIDC)
**Category:** Auth
**Complexity:** Medium

#### What it does
Instance can switch between authentication methods (email, LDAP, SAML, OIDC) with constraints. Only one method active at a time. Switching persists to database.

#### Specification
- Current authentication method stored in Settings table (key: `userManagement.authenticationMethod`)
- Allowed transitions: Email ↔ LDAP/SAML/OIDC; LDAP/SAML/OIDC → Email (via SSO-only intermediate); LDAP ↔ others via Email
- LDAP can switch to SAML/OIDC only via Email (cannot directly LDAP→SAML)
- Auth method change persists to config and database
- On LDAP disable, all LDAP identities removed from AuthIdentity table
- SSO login restrictions: SSO users (except owner) cannot login with email/password unless `settings.allowSSOManualLogin: true`
- `setCurrentAuthenticationMethod()` calls SettingsRepository to persist
- `reloadAuthenticationMethod()` reloads from database on startup or command

#### Implementation
**Entry point:** `packages/cli/src/sso.ee/sso-helpers.ts:14-26 — `setCurrentAuthenticationMethod()`;  28-45 — `reloadAuthenticationMethod()``
**Key files:**
- packages/cli/src/sso.ee/sso-helpers.ts:47-101 — authentication method helpers
- packages/cli/src/modules/ldap.ee/ldap.service.ee.ts:104-132 — LDAP config update with auth method switching
- packages/cli/src/modules/sso-saml/saml.service.ee.ts — SAML auth method integration
- packages/cli/src/modules/sso-oidc/oidc.service.ee.ts:78-85 — OIDC init and auth method setup
- packages/@n8n/config/src/configs/sso.config.ts:32-53 — ProvisioningConfig with scope provisioning options

#### Dependencies
- Depends on: F-081, F-083, F-084, F-085
- External: SettingsRepository, GlobalConfig, Container.get() for global access

#### Porting Notes
- Auth method is global singleton; affects all users
- Switching from LDAP disables all LDAP users (sets disabled=true or removes AuthIdentity)
- SSO method switching may require data migration (e.g., email identities to SAML identities)
- SAML/OIDC direct switching not yet supported (must go through Email intermediate)
- Config reloading from database needed after switching to pick up new method settings

---

### F-100: SSO Just-In-Time (JIT) Provisioning
**Category:** Auth
**Complexity:** High

#### What it does
Automatically creates user accounts on first SSO login if enabled. Users provisioned with default role and optional claims-based role provisioning.

#### Specification
- JIT provisioning enabled by default (`N8N_SSO_JUST_IN_TIME_PROVISIONING=true`)
- First login via SAML/OIDC creates user if not exists
- User created with default role (typically `global:member`)
- Optional claim-based provisioning: SSO claim specifies user's instance role or project roles
- `scopesProvisionInstanceRole` enabled → instance role from claim (e.g., 'global:admin')
- `scopesProvisionProjectRoles` enabled → project roles from claim (e.g., [{'projectId': '...', 'role': 'project:editor'}])
- Claim name configurable: `scopesInstanceRoleClaimName`, `scopesProjectsRolesClaimName`
- OAuth scope requested: `N8N_SSO_SCOPES_NAME` (default: 'n8n')
- User update on subsequent logins applies latest claims
- JIT provisioning only applies to SAML/OIDC (not LDAP, which has explicit sync)

#### Implementation
**Entry point:** `packages/cli/src/modules/provisioning.ee/provisioning.service.ee.ts — JIT provisioning logic`
**Key files:**
- packages/cli/src/modules/provisioning.ee/provisioning.service.ee.ts — ProvisioningService
- packages/cli/src/modules/sso-saml/saml.service.ee.ts — SAML attribute mapping and user creation
- packages/cli/src/modules/sso-oidc/oidc.service.ee.ts — OIDC claim extraction and user creation
- packages/@n8n/config/src/configs/sso.config.ts:57-76 — JIT and scopes provisioning config

#### Dependencies
- Depends on: F-083, F-084, F-085, F-115
- External: ProvisioningService, SAML/OIDC services for attribute/claim extraction, Role service

#### Porting Notes
- JIT is convenience feature; if disabled, users must be pre-created
- Claim-based role provisioning requires SSO system to include custom claims in token/assertion
- Default role for JIT users is configurable per SSO type
- User update on re-login allows role changes to propagate from SSO without manual intervention

---

### F-101: API Key Scopes (License-Gated)
**Category:** Auth
**Complexity:** Medium

#### What it does
Fine-grained scopes for API keys allowing least-privilege access. Without license, all user-available scopes granted.

#### Specification
- Scopes selected at API key creation time
- Scopes must be subset of scopes user's role has access to
- License check: if `feat:apiKeyScopes` not licensed, all available scopes for user's role used
- Available scopes differ by role: owner sees all scopes, member sees limited scopes
- `getApiKeyScopesForRole()` returns scopes available to a given role
- API key validation checks scope against request endpoint's required scope

#### Implementation
**Entry point:** `packages/cli/src/controllers/api-keys.controller.ts:125-134 — scope resolution with license check`
**Key files:**
- packages/cli/src/controllers/api-keys.controller.ts:39-68 — API key creation with scope validation
- packages/@n8n/permissions/src/public-api-permissions.ee.ts — `getApiKeyScopesForRole()` function
- packages/cli/src/services/public-api-key.service.ts:51-52 — scope validation

#### Dependencies
- Depends on: F-096, F-095
- External: License service, permission system

#### Porting Notes
- API key scopes are subset of role's scopes; cannot grant scope user doesn't have
- License gate means without license, all scopes granted (convenience for free tier)
- Invalid scopes (not in user's available list) rejected with BadRequestError

---

### F-102: Instance Owner & Admin Role Distinctions
**Category:** Auth
**Complexity:** Medium

#### What it does
Owner is global singleton with immutable role; Admin is elevated member role with specific scopes. Only owner can create/delete admins or change global roles.

#### Specification
- Owner role: `global:owner` — immutable singleton, cannot be changed or transferred
- Admin role: `global:admin` — elevated permissions, can manage users/roles (but cannot touch owner)
- Owner can promote/demote users to/from admin
- Admin cannot touch owner or other admin role changes (admin-to-admin changes prohibited)
- Owner deletion requires data transfer; personal workflows/credentials transferred to new owner
- Owner cannot be deleted without designating successor
- Global role change endpoint: `PATCH /users/:id/role` with license `feat:advancedPermissions` required

#### Implementation
**Entry point:** `packages/cli/src/controllers/users.controller.ts:341-374 — role change endpoint`
**Key files:**
- packages/cli/src/controllers/users.controller.ts:341-374 — role change with owner checks
- packages/@n8n/db/src/constants.ts:52-54 — role definitions
- packages/cli/src/controllers/users.controller.ts:162-166 — admin cannot reset owner password

#### Dependencies
- Depends on: F-090, F-115
- External: License (feat:advancedPermissions), RoleService, UserRepository

#### Porting Notes
- Owner protection: admin cannot demote owner, owner cannot demote self
- Role change validation: admin attempting to change admin role rejected
- Owner deletion cascade: personal project, workflows, credentials transferred per user request

---

### F-103: Database Connection Initialization with TypeORM
**Category:** Data
**Complexity:** Medium

#### What it does
Initializes a DataSource connection to either SQLite or PostgreSQL, manages connection state, and periodically pings the database to monitor connection health.

#### Specification
- Supports two database types: SQLite (pooled, file-based) and PostgreSQL
- Tracks connection state with `connected` and `migrated` boolean flags
- Configurable ping interval (via `N8N_DB_PING_TIMEOUT`) to verify DB availability
- Migrations are executed with transaction-per-migration model
- Connection timeout configurable with `connectTimeoutMS` for PostgreSQL
- Ping timeout defaults to 5000ms and can be overridden via environment variable

#### Implementation
**Entry point:** `packages/@n8n/db/src/connection/db-connection.ts:20-130 — `DbConnection` service class with init(), migrate(), close() methods`
**Key files:**
- packages/@n8n/db/src/connection/db-connection.ts:50-78 — init() and migrate() methods
- packages/@n8n/db/src/connection/db-connection.ts:92-129 — Ping mechanism with error recovery
- packages/@n8n/db/src/connection/db-connection-options.ts:34-44 — Database type detection and routing

#### Dependencies
- Depends on: none
- External: @n8n/typeorm (custom fork), @n8n/backend-common Logger, @n8n/config DatabaseConfig

#### Porting Notes
- Migration transactions are per-migration, not across all migrations
- Ping uses a race condition pattern: Promise.race() to timeout long-running connections
- SQLite uses WAL mode (Write-Ahead Logging), pool size configurable
- PostgreSQL supports SSL/TLS with custom CA, cert, key
- Table prefix is applied to all entities for multi-tenant deployments

---

### F-104: Database Configuration with SQLite and PostgreSQL Support
**Category:** Data
**Complexity:** Medium

#### What it does
Generates TypeORM DataSourceOptions with database-specific configuration for SQLite or PostgreSQL, including connection pooling, SSL/TLS, logging, and entity/migration registration.

#### Specification
- SQLite: pooled driver with 60s acquire timeout, 5s destroy timeout, WAL enabled
- PostgreSQL: configurable pool size, idle timeout, statement timeout, SSL options
- Supports custom table prefix for multi-tenancy
- Entities loaded from module registry for extensibility
- Logging can be disabled, enabled for all, or selective (query, error, warn, info, schema)
- Subscribers (TypeORM listeners) auto-registered
- No synchronize mode (migrations must be explicit)

#### Implementation
**Entry point:** `packages/@n8n/db/src/connection/db-connection-options.ts:16-118 — `DbConnectionOptions` service`
**Key files:**
- packages/@n8n/db/src/connection/db-connection-options.ts:71-85 — SQLite configuration
- packages/@n8n/db/src/connection/db-connection-options.ts:87-117 — PostgreSQL configuration with SSL
- packages/@n8n/db/src/connection/db-connection-options.ts:46-69 — Common options (logging, entities, migrations)

#### Dependencies
- Depends on: F-103
- External: @n8n/typeorm DataSourceOptions, @n8n/config (DatabaseConfig, InstanceSettingsConfig), @n8n/backend-common ModuleRegistry, node:tls TlsOptions

#### Porting Notes
- SSL configuration is optional; empty strings + rejectUnauthorized=true disables custom certs
- Postgres idleTimeoutMillis is passed as `extra.idleTimeoutMillis`
- SQLite database path is resolved from n8nFolder + config database path
- Migrations are DB-specific arrays (sqliteMigrations vs postgresMigrations)

---

### F-105: Custom Migration DSL with Database-Agnostic API
**Category:** Data
**Complexity:** High

#### What it does
Provides a TypeORM wrapper that executes migrations with wrapped context (MigrationContext), enabling a consistent DSL across SQLite and PostgreSQL migrations. Handles foreign key disabling for SQLite table recreation.

#### Specification
- Wraps all migrations with context containing queryRunner, logger, schemaBuilder, escape utilities
- For SQLite: can disable foreign keys (PRAGMA foreign_keys=OFF) during migration if `withFKsDisabled=true`
- Transactions: per-migration (each migration in its own transaction, unless FKs disabled)
- Batch operations: copyTable(), runInBatches() for large data moves
- DB-agnostic query escaping (columnName, tableName, indexName)
- Personalization survey JSON loading from disk during migration
- Logs migration start/end (skipped in test env)

#### Implementation
**Entry point:** `packages/@n8n/db/src/migrations/migration-helpers.ts:203-248 — `wrapMigration()` function`
**Key files:**
- packages/@n8n/db/src/migrations/migration-helpers.ts:136-201 — createContext() provides the DSL API
- packages/@n8n/db/src/migrations/migration-helpers.ts:64-85 — runDisablingForeignKeys() for SQLite
- packages/@n8n/db/src/migrations/migration-helpers.ts:102-134 — copyTable() batch copy with configurable batch size

#### Dependencies
- Depends on: F-103
- External: @n8n/backend-common Logger, @n8n/config GlobalConfig, @n8n/di Container, @n8n/typeorm QueryRunner, n8n-core InstanceSettings

#### Porting Notes
- Schema builder imported from './dsl' (separate file for table/index creation abstraction)
- Batch size defaults to 10 for copyTable, 100 for runInBatches
- Foreign key pragma only for SQLite; ignored if dbType !== 'sqlite'
- All migration classes must have `up()` method; `down()` is optional (reversible vs irreversible)
- withFKsDisabled disables transaction property via getter proxy

---

### F-106: Migration Types and Interfaces
**Category:** Data
**Complexity:** Low

#### What it does
Defines TypeScript types and interfaces for migration implementations, supporting both reversible and irreversible migrations.

#### Specification
- MigrationContext: type-safe context with 15 properties (logger, queryRunner, tablePrefix, dbType, schemaBuilder, etc.)
- BaseMigration: requires `up()` method, optional `down()`, optional `withFKsDisabled` flag
- ReversibleMigration extends BaseMigration with mandatory `down()` method
- IrreversibleMigration extends BaseMigration with no `down()` method
- Migration: function type matching TypeORM migration pattern
- Database type: 'postgresdb' | 'sqlite'

#### Implementation
**Entry point:** `packages/@n8n/db/src/migrations/migration-types.ts:1-71`
**Key files:**
- packages/@n8n/db/src/migrations/migration-types.ts:8-39 — MigrationContext interface
- packages/@n8n/db/src/migrations/migration-types.ts:43-66 — BaseMigration, ReversibleMigration, IrreversibleMigration

#### Dependencies
- Depends on: F-105
- External: @n8n/backend-common Logger, @n8n/typeorm QueryRunner, n8n-workflow (re-exported QueryFailedError)

#### Porting Notes
- Migration.prototype must have __n8n_wrapped marker to prevent double-wrapping
- Getter for transaction property checks withFKsDisabled at runtime
- MigrationFn is async function type accepting MigrationContext

---

### F-107: CredentialsEntity with Encryption, Sharing, and Resolver Support
**Category:** Data
**Complexity:** Medium

#### What it does
Stores encrypted credential data with metadata for name, type, sharing state, managed flag, and dynamic credential resolver configuration.

#### Specification
- Inherits from WithTimestampsAndStringId (id, createdAt, updatedAt)
- `data` field stores encrypted credential payload as text (JSON string)
- `name`: 3-128 characters, indexed (unique per scope)
- `type`: 128 chars max, indexed (e.g., 'oauth2', 'basicAuth')
- `isManaged`: boolean, indicates n8n-managed credential (e.g., OpenAI free credits) — immutable by users
- `isGlobal`: boolean, available to all users
- `isResolvable`: boolean, can be dynamically resolved by resolver
- `resolvableAllowFallback`: boolean, allow fallback to static credential if dynamic resolution fails
- `resolverId`: optional reference to dynamic credential resolver ID
- Relationship: OneToMany to SharedCredentials (sharing rules)
- toJSON() excludes `shared` relation

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/credentials-entity.ts:8-68`
**Key files:**
- packages/@n8n/db/src/entities/credentials-entity.ts:10-26 — Core fields (name, data, type)
- packages/@n8n/db/src/entities/credentials-entity.ts:28-62 — Sharing and resolver fields

#### Dependencies
- Depends on: F-123
- External: @n8n/typeorm decorators (Column, Entity, Index, OneToMany), class-validator (IsString, IsObject, Length)

#### Porting Notes
- `data` field is encrypted by CredentialsHelper (n8n-core Credentials class) — encryption happens outside entity layer
- `shared` relation is typed but omitted from toJSON() to prevent exposing sharing details in API responses
- Indexes on `type` for fast credential type lookups
- Validates name length at class level with @Length decorator

---

### F-108: Credential Decryption and Redaction in Service Layer
**Category:** Data
**Complexity:** High

#### What it does
Decrypts CredentialsEntity data using n8n-core Credentials class and optionally redacts sensitive fields (passwords, OAuth tokens) for security.

#### Specification
- decrypt() method wraps n8n-core Credentials.getData()
- Automatic redaction by default (includeRawData=false) — replaces sensitive values with `true` boolean
- Redacts: password fields, oauthTokenData, csrfSecret
- Returns empty object {} on CredentialDataError (logs error with credential ID, type)
- oauthTokenData is blanked to boolean `true` in API responses (never expose refresh tokens)
- unredact() method reconstructs original data by merging decrypted with redacted copy for UI previews

#### Implementation
**Entry point:** `packages/cli/src/credentials/credentials.service.ts:637-656 — decrypt() method`
**Key files:**
- packages/cli/src/credentials/credentials.service.ts:637-656 — decrypt() implementation
- packages/cli/src/credentials/credentials.service.ts:754-809 — redact() method
- packages/cli/src/credentials/credentials.service.ts:599-601 — OAuth token preservation during update

#### Dependencies
- Depends on: F-107, F-124
- External: n8n-core Credentials, CredentialDataError, @/credentials-helper createCredentialsFromCredentialsEntity

#### Porting Notes
- Encryption/decryption happens in n8n-core, not @n8n/db layer
- Redaction is security-critical: ensure sensitive fields list matches credential type definitions
- oauthTokenData update logic: if original has oauthTokenData, preserve it during credential edit (lines 599-601)
- Error handling: CredentialDataError indicates corrupt/unreadable encrypted data

---

### F-109: Credentials Repository with ListQuery Filtering and Sharing Subqueries
**Category:** Data
**Complexity:** High

#### What it does
TypeORM repository for querying credentials with advanced filtering by name, type, project, role, and user. Supports pagination, field selection, and sharing permission subqueries.

#### Specification
- findMany() / findManyAndCount(): query with ListQuery options (filter, select, take, skip, order)
- Default select: id, name, type, isManaged, createdAt, updatedAt, isGlobal, isResolvable, resolverId
- Optional relations: shared, shared.project, shared.project.projectRelations
- includeData flag: adds encrypted `data` field when requested
- Filters: name (LIKE), type (LIKE), projectId, withRole, user.id
- findAllGlobalCredentials(): queries isGlobal=true
- findAllPersonalCredentials(): queries shared.project.type='personal'
- getManyAndCountWithSharingSubquery(): subquery-based sharing check (combines credential + sharing permission in single query)
- Sharing options: scopes, projectRoles, credentialRoles, personalProjectOwnerId, onlySharedWithMe

#### Implementation
**Entry point:** `packages/@n8n/db/src/repositories/credentials.repository.ts:12-377`
**Key files:**
- packages/@n8n/db/src/repositories/credentials.repository.ts:24-40 — findMany() signature
- packages/@n8n/db/src/repositories/credentials.repository.ts:60-129 — toFindManyOptions() builds filter, select, relations
- packages/@n8n/db/src/repositories/credentials.repository.ts:230-262 — getManyAndCountWithSharingSubquery() two-query pattern
- packages/@n8n/db/src/repositories/credentials.repository.ts:267-376 — getManyQueryWithSharingSubquery() builds subquery

#### Dependencies
- Depends on: F-107, F-103
- External: @n8n/typeorm Repository, DataSource, In, Like, SelectQueryBuilder; @n8n/permissions Scope

#### Porting Notes
- Subquery pattern uses Container.get(SharedCredentialsRepository) — requires DI setup
- Sharing subquery includes nested relations (project.projectRelations.userId) for role-based filtering
- Pagination requires `id` in select even if not explicitly requested
- LIKE filters add wildcard % on both sides (for name) or just prefix (type can vary by use case)

---

### F-110: SharedCredentials Entity with Project-Based Access Control
**Category:** Data
**Complexity:** Medium

#### What it does
Junction table mapping credentials to projects with role-based access control, enabling multi-project credential sharing.

#### Specification
- Composite primary key: (credentialsId, projectId)
- role: CredentialSharingRole enum (e.g., 'credential:owner', 'credential:editor', 'credential:user')
- Timestamps: createdAt, updatedAt (via WithTimestamps)
- Relations: ManyToOne to CredentialsEntity, ManyToOne to Project
- toJSON() not overridden — all fields included
- Foreign keys: CASCADE delete on credential/project removal
- One credential → multiple projects (shared across teams/departments)

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/shared-credentials.ts:8-24`
**Key files:**
- packages/@n8n/db/src/entities/shared-credentials.ts:8-24 — Entity definition

#### Dependencies
- Depends on: F-107, F-120
- External: @n8n/typeorm decorators, @n8n/permissions CredentialSharingRole

#### Porting Notes
- Role field is string (TypeORM) but typed to CredentialSharingRole enum in @n8n/permissions
- Project relation enables hierarchical permission checks (user's project role determines credential access)
- No index on this table beyond composite PK; queries typically join via CredentialsEntity.shared

---

### F-111: Shared Credentials Repository with Subquery-Based Permission Checks
**Category:** Data
**Complexity:** High

#### What it does
TypeORM repository providing complex sharing permission queries, including role-based filtering, subqueries for credential ownership, and support for personal vs team projects.

#### Specification
- findByCredentialIds(): find sharings by role, returns credentials + project + project relations
- makeOwner(): upsert 'credential:owner' role for credentials in a project
- getFilteredAccessibleCredentials(): returns credential IDs user has access to via shared credentials
- findCredentialOwningProject(): finds the project that owns (has :owner role for) a credential
- buildSharedCredentialIdsSubquery(): creates subquery for filtering credentials by sharing permissions and roles
- getSharedPersonalCredentialsCount(): counts credentials shared from personal projects

#### Implementation
**Entry point:** `packages/@n8n/db/src/repositories/shared-credentials.repository.ts:10-377`
**Key files:**
- packages/@n8n/db/src/repositories/shared-credentials.repository.ts:36-49 — makeOwner() upsert
- packages/@n8n/db/src/repositories/shared-credentials.repository.ts:61-74 — getFilteredAccessibleCredentials() permission check
- packages/@n8n/db/src/repositories/shared-credentials.repository.ts:149-250+ — buildSharedCredentialIdsSubquery() (continues beyond visible range)

#### Dependencies
- Depends on: F-110, F-103
- External: @n8n/typeorm Repository, In, Not, SelectQueryBuilder; @n8n/permissions CredentialSharingRole, hasGlobalScope, PROJECT_OWNER_ROLE_SLUG

#### Porting Notes
- buildSharedCredentialIdsSubquery() is complex: filters by scopes, project roles, credential roles, personal project flag
- Permission model: User → ProjectRelation (with role) → Project → SharedCredentials → Credential
- Subquery essential for performance; direct JOINs would create Cartesian products with multiple role tables

---

### F-112: WorkflowEntity with Versioning, Tags, Sharing, and Pin Data
**Category:** Data
**Complexity:** High

#### What it does
Stores workflow definition (nodes, connections, settings) with version control, tagging, sharing, and pinned node data for quick reference.

#### Specification
- Inherits WithTimestampsAndStringId (id, createdAt, updatedAt)
- `name`: 1-128 chars, indexed (unique at DB level)
- `description`: optional text field
- `active`: boolean (deprecated, use activeVersionId instead)
- `isArchived`: soft-delete flag; archived workflows can still execute as sub-workflows but not edited
- `nodes`: JSON array of INode objects
- `connections`: JSON object (IConnections) mapping node connections
- `settings`: optional workflow settings (timeouts, error handlers)
- `staticData`: optional workflow-level persistent data (state)
- `meta`: optional workflow metadata (frontend state, UI preferences)
- `pinData`: optional simplified pin data (bypasses node execution, uses pinned values)
- `versionId`: string, identifies current version
- `activeVersionId`: nullable reference to WorkflowHistory versionId (which version is active)
- `versionCounter`: incremented on each change
- `triggerCount`: count of enabled trigger nodes (used for billing)
- `parentFolder`: optional ManyToOne to Folder (for organization)
- Relations: ManyToMany to TagEntity, OneToMany to SharedWorkflow, OneToMany to WorkflowTagMapping, OneToMany to TestRun

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/workflow-entity.ts:25-127`
**Key files:**
- packages/@n8n/db/src/entities/workflow-entity.ts:52-72 — Node, connections, settings, staticData (JSON columns)
- packages/@n8n/db/src/entities/workflow-entity.ts:100-112 — Version control fields
- packages/@n8n/db/src/entities/workflow-entity.ts:118-127 — Folder relation

#### Dependencies
- Depends on: F-123, F-118
- External: @n8n/typeorm decorators, n8n-workflow (INode, IConnections, IWorkflowSettings, WorkflowFEMeta), abstract-entity transformers

#### Porting Notes
- `active` field deprecated; activeVersionId=null means workflow inactive
- pinData uses conditional transformer (objectRetriever) to handle SQLite simple-json type
- versionCounter incremented on update (not managed by entity, done in service layer)
- triggerCount excludes error/disabled triggers (used for billing/plan enforcement)
- Folder relationship is soft-delete: onDelete='CASCADE' but workflow_entity doesn't have fk constraint in all DBs

---

### F-113: ExecutionEntity with Status, Retry Chain, and Flexible Data Storage
**Category:** Data
**Complexity:** High

#### What it does
Records workflow execution instances with status, timing, mode, retry tracking, and pointer to execution data (stored separately in ExecutionData).

#### Specification
- Uses @Generated() and @PrimaryColumn() for auto-incremented string IDs (idStringifier transformer)
- `finished`: boolean (deprecated, use status instead)
- `mode`: WorkflowExecuteMode enum (manual, automatic, etc.)
- `status`: ExecutionStatus (new/running/success/error/waiting/etc.)
- `retryOf`: optional reference to original execution ID if this is a retry
- `retrySuccessId`: optional reference to successful retry execution ID
- `createdAt`: execution enqueued time
- `startedAt`: nullable, time execution actually started (may differ from createdAt if queued)
- `stoppedAt`: nullable, execution end time (indexed for queries)
- `deletedAt`: soft-delete timestamp (DeleteDateColumn)
- `workflowId`: optional reference to WorkflowEntity
- `waitTill`: nullable date, used for paused/waiting executions
- `storedAt`: 'db' | 'fs' | 's3', indicates where execution data is stored
- Indexes: [workflowId, id], [waitTill, id], [finished, id], [workflowId, finished, id], [stoppedAt]
- Relation: OneToOne to ExecutionData, ManyToOne to WorkflowEntity, OneToMany to ExecutionMetadata, OneToOne to ExecutionAnnotation

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/execution-entity.ts:25-99`
**Key files:**
- packages/@n8n/db/src/entities/execution-entity.ts:31-54 — Core fields (id, finished, mode, status)
- packages/@n8n/db/src/entities/execution-entity.ts:56-81 — Timing fields (createdAt, startedAt, stoppedAt, waitTill, deletedAt)
- packages/@n8n/db/src/entities/execution-entity.ts:88-99 — Relations

#### Dependencies
- Depends on: F-123
- External: @n8n/typeorm decorators, n8n-workflow ExecutionStatus/WorkflowExecuteMode enums, custom idStringifier transformer

#### Porting Notes
- ExecutionData stored in separate table (ExecutionData) to enable flexible storage backends (db/fs/s3)
- waitTill used for schedule-based resumption (cron jobs, wait nodes)
- Retry chain: execution A (failed) → retryOf=A, execution B (success) → retrySuccessId=B
- storedAt='db'|'fs'|'s3' configurable; determines where binary/large execution data lives
- Indexes optimized for: filtering by workflowId+status, wait till resume queries, cleanup queries

---

### F-114: ExecutionData with Workflow Definition and Pin Data
**Category:** Data
**Complexity:** Medium

#### What it does
Stores execution runtime data (execution results) and workflow definition snapshot separately from ExecutionEntity, enabling independent storage backends and archival strategies.

#### Specification
- OneToOne relation to ExecutionEntity (cascade delete on execution removal)
- `data`: text field, execution runtime data (stringified IRunExecutionData)
- `workflowData`: JSON, workflow snapshot at execution time (IWorkflowBase without pinData or extended with simplified pinData)
- `executionId`: string primary key, foreign key to ExecutionEntity
- `workflowVersionId`: optional varchar(36), version ID of workflow at execution time

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/execution-data.ts:9-40`
**Key files:**
- packages/@n8n/db/src/entities/execution-data.ts:11-12 — data and workflowData columns
- packages/@n8n/db/src/entities/execution-data.ts:33-39 — ExecutionEntity relation

#### Dependencies
- Depends on: F-113
- External: @n8n/typeorm decorators, n8n-workflow IWorkflowBase, custom JsonColumn/idStringifier

#### Porting Notes
- data column stores execution context, node outputs, errors (flattened/stringified)
- workflowData includes node definitions but simplified pinData (ISimplifiedPinData) to avoid TS deep type issues
- executionId is both PK and FK; one-to-one with ExecutionEntity
- workflowVersionId nullable: null for manual/ad-hoc executions

---

### F-115: User Entity with Email, Password, MFA, and Auth Identities
**Category:** Data
**Complexity:** Medium

#### What it does
Stores user account information including authentication credentials (password, OAuth), MFA settings, personalization survey answers, and role assignment.

#### Specification
- Inherits WithTimestamps (createdAt, updatedAt)
- `id`: UUID primary key (PrimaryGeneratedColumn)
- `email`: 254 chars max, unique index, lowercase transformer, nullable
- `firstName`, `lastName`: optional strings (32 chars each)
- `password`: nullable (OAuth users have null password)
- `personalizationAnswers`: JSON object, nullable (survey responses)
- `settings`: JSON object, nullable (user preferences)
- `role`: ManyToOne to Role entity (lookup by roleSlug FK)
- `disabled`: boolean flag (prevents login)
- `mfaEnabled`: boolean, whether MFA active
- `mfaSecret`: nullable, TOTP secret (encrypted by n8n-core)
- `mfaRecoveryCodes`: simple-array type, comma-separated recovery codes
- `lastActiveAt`: nullable date
- `isPending`: computed property (not persisted), true if password=null AND no external auth AND not owner
- Relations: OneToMany AuthIdentity, OneToMany ApiKey, OneToMany SharedWorkflow, OneToMany SharedCredentials, OneToMany ProjectRelation
- Hooks: BeforeInsert/BeforeUpdate lowercases email and validates format
- toJSON() excludes password, mfaSecret, mfaRecoveryCodes
- createPersonalProjectName(): generates project name from firstName+lastName+email

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/user.ts:29-143`
**Key files:**
- packages/@n8n/db/src/entities/user.ts:34-81 — Core user fields
- packages/@n8n/db/src/entities/user.ts:96-106 — MFA fields
- packages/@n8n/db/src/entities/user.ts:82-94 — Email validation hook
- packages/@n8n/db/src/entities/user.ts:113-122 — isPending computed property

#### Dependencies
- Depends on: F-123
- External: @n8n/typeorm decorators, @n8n/permissions AuthPrincipal interface, isValidEmail validator, lowerCaser/objectRetriever transformers

#### Porting Notes
- Email lowercase + validation enforced at model level (prevents invalid emails at insert/update)
- isPending computed on AfterLoad/AfterUpdate: not persisted, computed from password+authIdentities+role
- MFA recovery codes stored as simple-array (comma-separated string in DB)
- toJSON() security: password/mfaSecret/mfaRecoveryCodes never leaked in API responses
- Role must exist before user insert (FK constraint)

---

### F-116: AuthIdentity Entity for OAuth/LDAP/SSO Integration
**Category:** Data
**Complexity:** Medium

#### What it does
Links users to external authentication providers (OAuth, LDAP, SAML) for single sign-on and federated authentication.

#### Specification
- Composite primary key: (providerId, providerType)
- `userId`: reference to User (ManyToOne)
- `providerId`: unique identifier from external provider (255 chars)
- `providerType`: enum type ('email', 'ldap', 'oauth2', etc.)
- Unique constraint: (providerId, providerType) — one provider identity per user per provider type
- Timestamps: createdAt, updatedAt (via WithTimestamps)
- Static factory method: AuthIdentity.create(user, providerId, providerType='ldap')

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/auth-identity.ts:8-37`
**Key files:**
- packages/@n8n/db/src/entities/auth-identity.ts:8-37 — Full entity definition

#### Dependencies
- Depends on: F-115, F-123
- External: @n8n/typeorm decorators, AuthProviderType enum from types-db

#### Porting Notes
- OAuth token storage: tokens NOT stored in AuthIdentity; they are encrypted and stored in Credentials entity
- This entity is for identity mapping only (who authenticated via which provider)
- providerId examples: GitHub username, Google account ID, LDAP distinguished name
- One user can have multiple AuthIdentities (e.g., both OAuth2 Google and LDAP)

---

### F-117: ApiKey Entity with Scopes and Audience
**Category:** Data
**Complexity:** Low

#### What it does
Stores API keys for programmatic access to n8n, with scope-based authorization and audience specification (public API vs internal).

#### Specification
- Inherits WithTimestampsAndStringId (id, createdAt, updatedAt)
- `userId`: reference to User (ManyToOne with CASCADE delete)
- `label`: user-friendly name for key (e.g., "Production API Key")
- `apiKey`: unique indexed string, the actual secret (stored in DB, not hashed — assume DB is secure)
- `scopes`: JSON array of ApiKeyScope strings (e.g., ['workflows:read', 'workflows:write'])
- `audience`: ApiKeyAudience enum (default 'public-api')
- Unique constraint: (userId, label) — one key label per user
- Table name: `user_api_keys`

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/api-key.ts:8-33`
**Key files:**
- packages/@n8n/db/src/entities/api-key.ts:8-33 — Entity definition

#### Dependencies
- Depends on: F-115, F-123
- External: @n8n/typeorm decorators, @n8n/permissions ApiKeyScope, n8n-workflow ApiKeyAudience enum

#### Porting Notes
- apiKey stored plaintext (not hashed); only revealed once at creation time
- Scopes in JSON format; validation happens in service layer
- audience field distinguishes public API keys from internal/dev audience
- Deletion via CASCADE: deleting user deletes all their keys

---

### F-118: TagEntity with ManyToMany Workflow and Folder Mapping
**Category:** Data
**Complexity:** Low

#### What it does
Stores user-defined tags (labels) that can be applied to workflows and folders for organization and categorization.

#### Specification
- Inherits WithTimestampsAndStringId (id, createdAt, updatedAt)
- `name`: 1-24 chars, unique indexed string
- ManyToMany relation to WorkflowEntity (via `workflows_tags` junction table)
- OneToMany to WorkflowTagMapping (alternative mapping, likely for EE features)
- OneToMany to FolderTagMapping (tags on folders)
- Class validators: IsString, Length(1, 24)

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/tag-entity.ts:9-25`
**Key files:**
- packages/@n8n/db/src/entities/tag-entity.ts:9-25 — Entity definition

#### Dependencies
- Depends on: F-123
- External: @n8n/typeorm decorators, class-validator IsString/Length

#### Porting Notes
- ManyToMany junction table auto-generated with joinColumn/inverseJoinColumn
- WorkflowTagMapping appears to be parallel mapping (maybe for soft-delete tags?)
- Tag names should be globally unique (across all users)

---

### F-119: WebhookEntity for Webhook HTTP Routing
**Category:** Data
**Complexity:** Medium

#### What it does
Maps HTTP webhook paths and methods to workflow nodes, enabling webhook-triggered executions with dynamic path support.

#### Specification
- Composite primary key: (webhookPath, method)
- `workflowId`: reference to workflow that owns this webhook
- `node`: node ID within the workflow that handles the webhook
- `method`: HTTP method enum (GET, POST, PUT, DELETE, etc.)
- `webhookId`: optional UUID for dynamic path webhooks (e.g., `/{{uuid}}/customer/:id`)
- `pathLength`: number of path segments (for routing optimization)
- `webhookPath`: complete path (e.g., `/user/123` or `/{{uuid}}/customer/:id`)
- Computed property `uniquePath`: combines webhookId + webhookPath for dynamic webhooks
- Computed property `cacheKey`: for webhook cache lookup (`webhook:METHOD-PATH`)
- Index: [webhookId, method, pathLength]
- Getter `isDynamic`: true if path contains `:param` segments
- Getter `staticSegments`: path parts without `:` prefix

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/webhook-entity.ts:4-57`
**Key files:**
- packages/@n8n/db/src/entities/webhook-entity.ts:4-57 — Entity definition and computed properties

#### Dependencies
- Depends on: F-123
- External: @n8n/typeorm decorators, n8n-workflow IHttpRequestMethods enum

#### Porting Notes
- No timestamps (not softdeleted)
- Webhook URL construction: `${instanceUrl}/webhook/{{webhookId}}/user/:id`
- staticSegments used for fast routing (filter by prefix match)
- isDynamic optimization: routes with parameters need different handling (regex match vs exact)
- webhookId + webhookPath composite uniqueness: same path can exist for different webhooks

---

### F-120: ProjectEntity with Team/Personal Distinction and Nested Access Control
**Category:** Data
**Complexity:** Medium

#### What it does
Represents workspaces/teams that own workflows and credentials, with creator tracking and optional icons/descriptions. Enables multi-project credential and workflow sharing.

#### Specification
- Inherits WithTimestampsAndStringId (id, createdAt, updatedAt)
- `name`: 255 chars, workspace/team display name
- `type`: 'personal' | 'team' — personal project per user, team projects shared
- `icon`: nullable JSON object with type ('emoji'|'icon') and value
- `description`: optional 512-char description
- `creatorId`: nullable user ID (who created the project)
- Relations: OneToMany ProjectRelation (user memberships), OneToMany SharedCredentials, OneToMany SharedWorkflow, OneToMany ProjectSecretsProviderAccess, OneToMany Variables, ManyToOne User (creator)
- Foreign key on creatorId: SET NULL on user deletion

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/project.ts:11-46`
**Key files:**
- packages/@n8n/db/src/entities/project.ts:11-46 — Entity definition

#### Dependencies
- Depends on: F-123
- External: @n8n/typeorm decorators

#### Porting Notes
- Personal projects: auto-created one per user on signup
- Team projects: manually created, managed via ProjectRelation + Role system
- CreatorId SET NULL handles user deletion gracefully (project persists, creator unknown)
- icon/description optional but JSON structure defined (Emoji or icon-based branding)

---

### F-121: SettingsEntity for Key-Value Configuration Store
**Category:** Data
**Complexity:** Low

#### What it does
Stores instance-level settings as key-value pairs, with a loadOnStartup flag to indicate settings that should be cached/initialized on startup.

#### Specification
- `key`: string, primary key (e.g., 'instanceUrl', 'license', 'features')
- `value`: stored as JSON string but typed as string in entity (actual type varies by key)
- `loadOnStartup`: boolean flag (true = load into memory cache on init)

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/settings.ts:10-20`
**Key files:**
- packages/@n8n/db/src/entities/settings.ts:10-20 — Entity definition

#### Dependencies
- Depends on: F-103
- External: @n8n/typeorm decorators

#### Porting Notes
- Value always stored as string; caller responsible for JSON.parse()
- No timestamps (settings updated in place without tracking)
- loadOnStartup used for performance: high-frequency settings cached in memory
- Used for license keys, feature flags, global configuration

---

### F-122: Variables Entity with Project Scoping for Environment Variables
**Category:** Data
**Complexity:** Low

#### What it does
Stores environment variable-like key-value pairs, scoped to projects or global. Used for dynamic configuration in workflows.

#### Specification
- Inherits WithStringId (id only, no timestamps)
- `key`: text, variable name (e.g., 'API_TOKEN', 'DATABASE_URL')
- `type`: text, default 'string' (allow typing: string, number, boolean, etc.)
- `value`: text, variable value (encrypted or plaintext depending on n8n configuration)
- `project`: ManyToOne to Project, nullable (null = global variable)
- Foreign key: CASCADE delete on project removal
- No indexes besides PK

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/variables.ts:6-23`
**Key files:**
- packages/@n8n/db/src/entities/variables.ts:6-23 — Entity definition

#### Dependencies
- Depends on: F-120, F-123
- External: @n8n/typeorm decorators

#### Porting Notes
- Global variables: project=null, key scoped to globally
- Project variables: project=projectId, key scoped to project (multiple projects can have same key)
- Value NOT encrypted at entity level (encryption handled elsewhere if needed)
- type field allows future support for typed variables (number casting, boolean parsing)

---

### F-123: Abstract Entity Base Classes with ID Generation and Timestamps
**Category:** Data
**Complexity:** Low

#### What it does
Provides reusable base classes for common entity patterns (string IDs, timestamps, JSON columns).

#### Specification
- WithStringId: @PrimaryColumn() id with @BeforeInsert() auto-generates nanoid if missing
- WithCreatedAt: @CreateDateColumn() with precision=3, default to CURRENT_TIMESTAMP
- WithUpdatedAt: @UpdateDateColumn() with @BeforeUpdate() manual date set
- WithTimestamps: combines CreatedAt + UpdatedAt
- WithTimestampsAndStringId: combines StringId + Timestamps (most common base)
- JsonColumn(): decorator factory for type-safe JSON columns (simple-json for SQLite, json for PostgreSQL)
- DateTimeColumn(): decorator for database-specific datetime types (timestamptz for PostgreSQL, datetime for SQLite)
- BinaryColumn(): decorator for binary data (bytea for PostgreSQL, blob for SQLite)
- dbType constant: resolved at module init from @n8n/config

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/abstract-entity.ts:14-102`
**Key files:**
- packages/@n8n/db/src/entities/abstract-entity.ts:58-71 — WithStringId mixin
- packages/@n8n/db/src/entities/abstract-entity.ts:30-42 — JsonColumn factory
- packages/@n8n/db/src/entities/abstract-entity.ts:37-42 — DateTimeColumn factory
- packages/@n8n/db/src/entities/abstract-entity.ts:51-55 — Timestamp syntax (database-specific)

#### Dependencies
- Depends on: none
- External: @n8n/config GlobalConfig, @n8n/di Container, @n8n/utils generateNanoId, n8n-core Class type

#### Porting Notes
- dbType resolved once at module init; dbType constant used throughout for conditional logic
- Timestamp precision=3: millisecond granularity (CURRENT_TIMESTAMP(3) in PostgreSQL, STRFTIME in SQLite)
- generateNanoId() auto-generates 12-char alphanumeric IDs (vs UUID for user PK)
- Mixins use generics and Function type to allow composition without diamond problem

---

### F-124: Credential Encryption/Decryption Integration with n8n-core
**Category:** Data
**Complexity:** High

#### What it does
Integrates n8n-core Credentials class for encryption/decryption of credential data, providing symmetric key-based encryption with lazy-load key resolution.

#### Specification
- Encryption handled by n8n-core Credentials class (outside DB layer)
- CredentialsEntity.data stores encrypted JSON as string
- Symmetric encryption key sourced from environment/config (CREDENTIALS_ENCRYPTION_KEY or derived)
- Decryption: Credentials.getData() deserializes + decrypts data field
- OAuth token data (access token, refresh token) stored in decrypted.oauthTokenData
- Encryption/decryption transparent to DB layer — entity just stores encrypted blob
- CredentialDataError thrown on decrypt failure (corrupt data, wrong key)

#### Implementation
**Entry point:** `packages/cli/src/credentials/credentials.service.ts:637-656 — decrypt() call to n8n-core Credentials`
**Key files:**
- packages/cli/src/credentials-helper.ts:82-94 — CredentialsHelper service instantiation
- packages/cli/src/credentials/credentials.service.ts:637-656 — decrypt() wrapper

#### Dependencies
- Depends on: F-107
- External: n8n-core Credentials class, CredentialDataError exception

#### Porting Notes
- Encryption key management is outside DB scope (typically environment variable)
- Decryption always performed on read (no caching); encryption always performed on write
- CredentialDataError handling: error is logged, empty object returned (prevents cascade failures)
- OAuth token refresh: oauthTokenData preserved during credential updates (lines 599-601 in credentials.service)

---

### F-125: Credential Update Workflow with OAuth Token Preservation
**Category:** Data
**Complexity:** High

#### What it does
Updates credential data while preserving sensitive OAuth token data that may be stored alongside user-configured credential fields.

#### Specification
- User submits credential update (password, API key, etc.)
- Service decrypts existing credential to extract oauthTokenData
- Decrypted data merged with user update (user data overwrites old, oauthTokenData preserved)
- unredact() step: reconstructs full data by copying back oauthTokenData from original before re-encryption
- Final encrypted data saved to DB
- OAuth tokens never exposed to frontend, only marked as `true` in API responses

#### Implementation
**Entry point:** `packages/cli/src/credentials/credentials.service.ts:558-617 — update() method`
**Key files:**
- packages/cli/src/credentials/credentials.service.ts:563-574 — Decrypt existing + merge new data
- packages/cli/src/credentials/credentials.service.ts:588-601 — Unredact and preserve oauthTokenData
- packages/cli/src/credentials/credentials.service.ts:658-667 — Update and persist

#### Dependencies
- Depends on: F-107, F-124
- External: CredentialsRepository.update(), Credentials encryption

#### Porting Notes
- Critical: if decryptedData.oauthTokenData exists, must preserve it in updateData (line 601)
- unredact() step ensures tokens don't get overwritten during user edits
- Merge strategy: shallow merge (top-level), oauthTokenData not deep-merged

---

### F-126: Credential Redaction for API Responses
**Category:** Data
**Complexity:** Medium

#### What it does
Replaces sensitive credential fields with boolean `true` for security, preventing accidental exposure of passwords, API keys, OAuth tokens in API responses or logs.

#### Specification
- redact() method iterates through decrypted credential data
- Sensitive fields: password fields (matching patterns), oauthTokenData, csrfSecret, other protected keys
- Redacted values replaced with `true` (boolean, not string)
- CREDENTIAL_BLANKING_VALUE constant defines the redacted value
- Used in getOne(), getMany() API responses
- OAuth tokens blanked to `true` to indicate "token data present but not shown"
- Inverse: unredact() restores actual values for internal processing

#### Implementation
**Entry point:** `packages/cli/src/credentials/credentials.service.ts:754-809 — redact() method`
**Key files:**
- packages/cli/src/credentials/credentials.service.ts:754-809 — Full redaction logic
- packages/cli/src/credentials/credentials.service.ts:637-644 — decrypt() with optional redaction

#### Dependencies
- Depends on: F-108
- External: NodeHelpers.getCredentialFields() to identify sensitive fields, displayParameter() for UI hints

#### Porting Notes
- Redaction is security-critical: ensure all credential types' sensitive fields are in the protected list
- Blanking value is boolean `true`, not string 'true' (important for JSON serialization)
- redact() is applied in service layer before returning to API, not at entity level

---

### F-127: ExecutionRepository with Complex Filtering, Status Updates, and Batch Operations
**Category:** Data
**Complexity:** High

#### What it does
TypeORM repository for execution queries with advanced filtering (status, workflow, metadata, date ranges), status updates with conditions, and bulk operations like deletion and binary data storage location changes.

#### Specification
- findMany() / findManyAndCount(): query with ExecutionEntity + ExecutionData relations
- Filters: id, finished, mode, retryOf, retrySuccessId, status (array), workflowId, waitTill, metadata (key-value array), date ranges (startedAfter, startedBefore)
- Advanced queries: parseFiltersToQueryBuilder(), SelectQueryBuilder for custom WHERE clauses
- updateExecution() / updateExecutionData(): conditional updates based on ExecutionStatus requirements
- upsert() for retry chain setup
- findMultipleExecutions(): fetch multiple by IDs with data relations
- deleteExecutions(): bulk soft-delete via custom SQL
- findReferencing(): find executions referencing another (for retry chains)
- getWaitingExecutions(): query for waitTill <= now (resumable executions)
- countExecutions(): aggregation queries with optional filtering
- ExecutionDeletionCriteria: complex filter object for deletion workflows

#### Implementation
**Entry point:** `packages/@n8n/db/src/repositories/execution.repository.ts:1-100+ (extends to 1186 lines)`
**Key files:**
- packages/@n8n/db/src/repositories/execution.repository.ts:96-150+ — parseFiltersToQueryBuilder() filter logic
- packages/@n8n/db/src/repositories/execution.repository.ts:1-88 — Type definitions (IGetExecutionsQueryFilter, UpdateExecutionConditions)

#### Dependencies
- Depends on: F-113, F-103
- External: @n8n/typeorm Repository/QueryBuilder, n8n-workflow ExecutionStatus/ErrorReporter, @n8n/backend-common Logger

#### Porting Notes
- Filtering is multi-stage: typeORM filters (status, workflowId) + custom QB filters (metadata key-value matching)
- Metadata filtering: each item has key, value, optional exactMatch flag
- Execution data storage: data column may be empty if storedAt='fs'|'s3' (data retrieved from external storage)
- Retry logic: retryOf/retrySuccessId form a chain; findReferencing() used to update retry outcomes

---

### F-128: WorkflowRepository with Complex Permission Queries and Folder/Tag Navigation
**Category:** Data
**Complexity:** High

#### What it does
TypeORM repository for workflow queries with role-based filtering, folder/tag relationships, and permission subqueries for multi-project access control.

#### Specification
- get() / findOne(): single workflow with optional relations
- findMany() / findManyAndCount(): query with sharing permissions, folders, tags
- Advanced filtering: name (LIKE), tags, workflowId, activeSince (version date), projectId
- Sharing subqueries: getWorkflowsWithSharedCredentials(), getWorkflowsWithoutSharedCredentials()
- Folder navigation: getWorkflowsInFolder(), getChildrenInFolder() (folders only)
- Tag operations: getTags(), countByTags()
- Permission checks: getManyByIds() with sharing relations, buildQuery() for role-based filtering
- getAllActiveIds(): returns IDs of active (activeVersionId != null) workflows for trigger registration
- Folder union queries: listFoldersAndWorkflows() returns combined folder+workflow rows

#### Implementation
**Entry point:** `packages/@n8n/db/src/repositories/workflow.repository.ts:57-150+ (extends to 1501 lines)`
**Key files:**
- packages/@n8n/db/src/repositories/workflow.repository.ts:68-76 — get() method
- packages/@n8n/db/src/repositories/workflow.repository.ts:78-82 — getAllActiveIds()

#### Dependencies
- Depends on: F-112, F-103
- External: @n8n/typeorm Repository/QueryBuilder, FolderRepository dependency, WorkflowHistoryRepository dependency

#### Porting Notes
- Folder/workflow union queries use UNION SQL for combined list view
- getAllActiveIds() critical for trigger registration (only active workflows can trigger)
- Sharing queries use nested JOINs: Workflow → SharedWorkflow → Project → ProjectRelation → User + Role
- Tag relations: ManyToMany (workflows_tags junction table)

---

### F-129: ExecutionRepository Retry Chain and Status Update Logic
**Category:** Data
**Complexity:** High

#### What it does
Manages execution retry relationships and conditional status updates, allowing retries to link back to original execution and original execution to reference successful retry.

#### Specification
- Retry chain: executionA (failed) → retryOf='executionA.id', executionB (success) → retrySuccessId='executionB.id' (in executionA)
- updateExecution(): updates execution record with optional status/data, validates current status (requireStatus, requireNotFinished, requireNotCanceled)
- findReferencing(): finds executions referencing given ID via retryOf or retrySuccessId
- upsert for retry setup: executionB updates executionA.retrySuccessId in transaction
- Status progression: running → success|error|timeout|etc
- Update conditions: can enforce pre-condition status check, prevent updates if finished, prevent updates if cancelled

#### Implementation
**Entry point:** `packages/@n8n/db/src/repositories/execution.repository.ts:66-94 — UpdateExecutionConditions interface`
**Key files:**
- packages/@n8n/db/src/repositories/execution.repository.ts:66-94 — UpdateExecutionConditions type
- Search for `updateExecution()` in execution.repository (implementation beyond visible range)

#### Dependencies
- Depends on: F-127, F-113
- External: @n8n/typeorm UpdateResult, n8n-workflow ExecutionStatus

#### Porting Notes
- Retry chain is acyclic graph: A→B (retryOf), A.retrySuccessId=B (success link)
- Status validation prevents invalid transitions (e.g., updating finished execution)
- Transactions ensure atomic retry chain setup

---

### F-130: Shared Workflow Entity and Repository for Multi-Project Workflow Sharing
**Category:** Data
**Complexity:** Medium

#### What it does
Maps workflows to projects with role-based access control, analogous to SharedCredentials, enabling workflows to be shared across teams/projects.

#### Specification
- Composite primary key: (workflowId, projectId)
- `role`: WorkflowSharingRole enum (workflow:owner, workflow:editor, workflow:viewer)
- Relations: ManyToOne to WorkflowEntity, ManyToOne to Project
- Timestamps: createdAt, updatedAt
- Foreign keys: CASCADE delete on workflow/project removal
- Similar to SharedCredentials: one workflow → multiple projects

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/shared-workflow.ts (inferred from references; full file not read)`
**Key files:**
- Shared workflow repository in packages/@n8n/db/src/repositories/shared-workflow.repository.ts (inferred)

#### Dependencies
- Depends on: F-112, F-120, F-123
- External: @n8n/typeorm decorators, @n8n/permissions WorkflowSharingRole

#### Porting Notes
- Parallel to SharedCredentials; same permission model
- Workflows can be shared at project level (all users in a project have role access)

---

### F-131: Project Relationship Entity and Role-Based Access Control
**Category:** Data
**Complexity:** Medium

#### What it does
Links users to projects with assigned roles (owner, editor, member), enabling project-level permission hierarchy.

#### Specification
- Foreign keys: userId (User), projectId (Project), roleId/roleSlug (Role)
- Roles: project owner, editor, member, etc. (defined in Role entity)
- One user → multiple projects, one project → multiple users (ManyToMany via ProjectRelation)
- User role in project determines access to all project resources (workflows, credentials, variables)

#### Implementation
**Entry point:** `packages/@n8n/db/src/entities/project-relation.ts (not fully read)`
**Key files:**
- Inferred from SharedCredentialsRepository joining on ProjectRelation

#### Dependencies
- Depends on: F-120, F-090, F-123
- External: @n8n/typeorm decorators, @n8n/permissions Role entity

#### Porting Notes
- Project role determines resource access: owner=full, editor=modify, member=read-only
- SharedCredentials/SharedWorkflow roles are independent of project role
- Permission check flow: User → ProjectRelation.role → can access project → can access shared resources

---

### F-132: Workflow Canvas Rendering with Vue Flow
**Category:** Frontend
**Complexity:** High

#### What it does
Renders an interactive node-based workflow graph using @vue-flow/core. Displays nodes as draggable elements on a canvas with edges (connections) between them. Supports panning, zooming, viewport management, and auto-fit view functionality.

#### Specification
- Canvas uses vue-flow v1.48.0 as core graph engine
- Nodes are rendered via VueFlow component with custom Node/Edge templates
- Supports multiple node render types: Default, StickyNote, AddNodes, ChoicePrompt
- Viewport initialized with optional transform (pan/zoom state)
- Grid-based positioning with GRID_SIZE = 8px spacing
- Edges support main and non-main connection types (visualized with/without dashes)
- MiniMap component shows global workflow overview
- Background with striped pattern for visual context
- Node positions are in XY coordinate space

#### Implementation
**Entry point:** `packages/frontend/editor-ui/src/features/workflows/canvas/components/Canvas.vue:44 — `VueFlow` component initialization`
**Key files:**
- Canvas.vue:174-203 — VueFlow hook setup with nodes, edges, selection, viewport management
- canvas.types.ts:1-151 — CanvasNode, CanvasConnection, CanvasNodeData type definitions
- canvas.eventBus.ts:1-5 — Event bus for canvas-wide events (fitView, selection, tidyUp)
- useCanvasLayout.ts:52-100 — Layout algorithm using Dagre for auto-positioning nodes

#### Dependencies
- Depends on: none
- External: - @vue-flow/core v1.48.0 (core graph library)
- @vue-flow/minimap v1.5.4 (viewport overview)
- @dagrejs/dagre v1.1.4 (hierarchical layout algorithm)

#### Porting Notes
- Vue Flow provides abstractions for pan/zoom; custom rendering via Node/Edge slot components
- Event system is critical — canvas emits ~20+ events for parent orchestration
- Viewport state persisted in store and passed as prop; coordinate system is XY relative to canvas origin
- Node dimensions auto-calculated; layout algorithm respects sticky notes as fixed elements

---

### F-133: Node Drag-and-Drop onto Canvas
**Category:** Frontend
**Complexity:** Medium

#### What it does
Users can drag nodes from the node creator panel and drop them onto the canvas, creating new workflow nodes at the drop position. Supports keyboard shortcut-triggered node creator (Cmd+K).

#### Specification
- Node creator opens as a right-side panel with search and categorized node browser
- Drag event dispatches with position data; canvas listens for 'drag-and-drop' event
- Drop position snapped to grid (GRID_SIZE = 8px)
- New node inserted at drop location via newNodeInsertPosition store ref
- Node creator keyboard shortcut: Cmd+K (configured in useKeybindings)
- Node selection automatically switches to newly created node
- Supports adding nodes in selection (multi-select mode tracks hasRangeSelection)

#### Implementation
**Entry point:** `Canvas.vue:118 — 'drag-and-drop' event emission; NodeCreator.vue:1-40 — drag source`
**Key files:**
- Canvas.vue:450-470 — onDragAndDrop handler sets newNodeInsertPosition
- NodeCreator.vue:43-68 — useNodeCreatorStore, state management for panel visibility
- nodeCreator.store.ts — Pinia store with showScrim, setActions, setMergeNodes
- NodeCreator.vue:71-120 — DRAG_EVENT_DATA_KEY constant and mouse event handling
- canvas.utils.ts — GRID_SIZE constant for snap-to-grid logic

#### Dependencies
- Depends on: F-132, F-145
- External: - @vueuse/core (onClickOutside for dismissing panel)
- n8n-workflow (NodeConnectionTypes for drag validation)

#### Porting Notes
- Drag-and-drop is browser native; n8n wraps it with position tracking
- Node creator is a separate, independently closeable panel; visibility managed via store
- Position data passed to parent (Canvas) emits 'drag-and-drop' with XYPosition
- Grid snap applied during node creation, not during drag visual feedback

---

### F-134: Node Connection (Edge) Creation via Handles
**Category:** Frontend
**Complexity:** High

#### What it does
Users click on node handles (input/output ports) and drag to create connections (edges) between nodes. Supports validation (max connections, node connection types), visual feedback during drag, and cancellation.

#### Specification
- Nodes expose input/output handles (CanvasConnectionPort[])
- Handles are rendered via CanvasHandleRenderer for each port
- OnConnect event from vue-flow validates connection, emits 'create:connection' event
- Connection line follows mouse during drag (CanvasConnectionLine component)
- Validates: target connection type matches, max connections not exceeded, connection not to self
- Main connections (solid line), non-main connections (dashed line)
- Edge toolbar appears on hover with add/delete buttons
- Status badges on edges: 'success', 'error', 'pinned', 'running'

#### Implementation
**Entry point:** `Canvas.vue:850-920 — onConnect, onConnectStart, onConnectEnd handlers`
**Key files:**
- CanvasEdge.vue:1-80 — Edge rendering with toolbar, status visualization
- CanvasHandleRenderer.vue — Renders all input/output handles for a node
- CanvasConnectionLine.vue — Live connection line during drag
- CanvasEdgeToolbar.vue — Add/delete edge UI
- canvas.types.ts:27-34 — CanvasConnectionPort type definition
- Canvas.vue:106-108 — 'create:connection:start/end/cancelled' events

#### Dependencies
- Depends on: F-132
- External: - @vue-flow/core (Connection type, Handle component, useVueFlow composable)
- n8n-workflow (NodeConnectionTypes enum, connection type validation)

#### Porting Notes
- Handles are positioned dynamically based on port count (CanvasElementPortWithRenderData)
- Connection validation delegated to parent via 'create:connection' event — canvas doesn't validate, parent workflow store does
- Edge toolbar uses delayed hover (600ms) to avoid flicker
- Connection data includes full CanvasConnectionPort with type/index for later reference

---

### F-135: Node Configuration Panel (NDV - Node Details View)
**Category:** Frontend
**Complexity:** High

#### What it does
Displays detailed node settings when a node is selected. Shows inputs/outputs, parameters, credentials, webhooks, and execution results in draggable panels.

#### Specification
- NDV triggered by clicking node or Enter key on selected node (Canvas.vue:342)
- Main panel contains NodeSettings (tabs for Settings, Input, Output, Webhook)
- Three draggable sub-panels: InputPanel (input data), MainPanel (node config), OutputPanel (result data)
- Tracks active node via useNDVStore.activeNodeName
- Settings include node credentials, parameters, sub-connections
- Supports inline editing with real-time parameter updates
- Execution tabs show previous run data indexed by run/branch
- Read-only mode when workflow is executing or in production mode

#### Implementation
**Entry point:** `NodeDetailsView.vue:1-100 — Main NDV component; Canvas.vue:889 — onSetNodeActivated emits 'update:node:activated'`
**Key files:**
- NodeDetailsView.vue:1-150 — Parent NDV with draggable panels (NDVDraggablePanels)
- NodeSettings.vue:1-120 — Settings tab with parameters, credentials, webhooks
- ndv.store.ts:38-120 — useNDVStore with activeNodeName, input/output panel state
- OutputPanel.vue — Displays execution result data in table/schema/JSON views
- InputPanel.vue — Shows input data from previous node execution
- ParameterInputList.vue — Renders node parameter inputs with type-aware controls

#### Dependencies
- Depends on: F-132
- External: - Element Plus (modal, dialog, panels)
- @n8n/design-system (N8nTabs, N8nIcon, form controls)
- n8n-workflow (NodeHelpers for parameter validation, NodeConnectionTypes)

#### Porting Notes
- NDV is modal-like but rendered within workflow editor layout
- Draggable panels use native Vue drag API; dimensions persisted per node type
- Input/output data display modes (table, schema, JSON) toggled via localStorage
- Real-time parameter updates emit 'valueChanged' event; parent handles validation and state update
- Credentials handled separately via NodeCredentials component with async loading

---

### F-136: Expression Editor (CodeMirror 6 Integration)
**Category:** Frontend
**Complexity:** High

#### What it does
Modal and inline code editors for entering n8n expressions (JavaScript-like syntax). Provides syntax highlighting, autocomplete, error detection, and variable/function reference.

#### Specification
- CodeMirror 6 editor with custom n8n expression language support
- Supports two modes: modal (ExpressionEditorModal) and inline (InlineExpressionEditor)
- Autocomplete suggests workflow variables, functions, node outputs
- Real-time expression validation and error highlighting
- Resolvable segments ({{ }}) parsed and highlighted
- Expression parsing with 300ms debounce for performance
- Keyboard shortcuts: Escape to close, Enter to confirm
- Integration with parameter fields via click-to-edit interaction

#### Implementation
**Entry point:** `useExpressionEditor.ts:55-100 — Composable hook for editor initialization`
**Key files:**
- useExpressionEditor.ts:55-100 — Main composable for CodeMirror setup, validation, autocomplete
- ExpressionEditorModalInput.vue — Modal editor component for complex expressions
- InlineExpressionEditorInput.vue — Inline editor for quick edits
- expressionCloseBrackets.ts — Plugin for auto-closing brackets
- resolvableHighlighter.ts — Syntax highlighter for {{ }} expressions
- completions/ — Autocomplete providers (workflow, functions, node outputs)

#### Dependencies
- Depends on: F-135
- External: - @codemirror/core v6+ (state, view, language, autocomplete, lint, search)
- @codemirror/lang-javascript — JS syntax support
- n8n-workflow Expression class for evaluation

#### Porting Notes
- Editor not a standalone component; composable injected into parameter fields
- Expression parsing detects {{ }} segments; only segments are resolvable/validated
- Autocomplete context depends on target node parameter — enables accurate suggestions
- Error messages include position/context for user debugging
- Debounced onChange prevents excessive validation during typing

---

### F-137: Node Execution Results Overlay
**Category:** Frontend
**Complexity:** Medium

#### What it does
Displays execution status (running, success, error, waiting) and result data on nodes during and after workflow execution. Shows execution badges, error indicators, run count.

#### Specification
- Execution status rendered via CanvasNodeStatusIcons component
- Nodes show: running spinner, execution status (success/error/waiting), run iteration count
- Error icons link to error details in NDV
- Pinned data indicator shows count of pinned data items
- Run data visibility toggled in OutputPanel
- Status updated via execution event bus (nodeExecuteBefore, nodeExecuteAfter events)
- Waiting state displayed when node waits for webhook/external trigger
- Color-coded: green (success), red (error), yellow (waiting), blue (running)

#### Implementation
**Entry point:** `CanvasNodeStatusIcons.vue — Status icon rendering for each node`
**Key files:**
- CanvasNodeStatusIcons.vue — Renders execution status icons (spinner, badges)
- CanvasNode.vue:85-100 — Node data includes execution status, runData, issues
- canvas.types.ts:124-133 — CanvasNodeData.execution and .runData structure
- usePushConnection/handlers/nodeExecuteAfter.ts — Updates node execution state
- useCanvasNode.ts:59-66 — Computed accessors for executionStatus, executionRunning, executionWaiting

#### Dependencies
- Depends on: F-135, F-011
- External: - @n8n/design-system (icons, spinners)
- n8n-workflow (ExecutionStatus enum)

#### Porting Notes
- Status icons overlay on node UI (toolbar); color/icon determined by execution state
- Run data linked to specific execution run/branch index — enables historical result viewing
- Error state persists until next execution or manual clear
- Waiting state triggered by serverless/webhook trigger nodes awaiting external events

---

### F-138: Workflow Run Button with Trigger Selection
**Category:** Frontend
**Complexity:** Medium

#### What it does
Button to start workflow execution. Shows execution progress, handles multiple trigger nodes with dropdown selector, shows waiting for webhook state.

#### Specification
- Button displays "Execute Workflow" text
- Loading state during execution (spinner icon)
- Keyboard shortcut: Cmd+Enter (Meta+Enter)
- Dropdown for selecting trigger node if multiple triggers exist
- Disabled when workflow is executing or unsupported configuration
- Shows "Waiting for trigger event" when webhook trigger is active
- Supports selecting which trigger node to use for execution
- Emits 'execute' and 'selectTriggerNode' events

#### Implementation
**Entry point:** `CanvasRunWorkflowButton.vue:1-80 — Run button component`
**Key files:**
- CanvasRunWorkflowButton.vue:20-80 — Button props, dropdown rendering
- CanvasControlButtons.vue — Container for all canvas control buttons
- Canvas.vue:1116 — 'run:workflow' event emission
- Canvas.vue:335-350 — Keyboard shortcut mapping for Cmd+Enter

#### Dependencies
- Depends on: F-132, F-005
- External: - @n8n/design-system (N8nButton, N8nActionDropdown)
- @n8n/i18n (internationalization strings)

#### Porting Notes
- Button state entirely derived from props — stateless presentation component
- Trigger node selection stored in parent workflow store; button reflects selection
- Disabled state logic: executing OR !triggerNodes.length OR readOnly
- Tooltip includes keyboard shortcut hint via KeyboardShortcutTooltip wrapper

---

### F-139: Manual Node Execution (Run Button on Toolbar)
**Category:** Frontend
**Complexity:** Medium

#### What it does
Executes a single node without running full workflow. Used for testing individual node configurations and inspecting output.

#### Specification
- Run icon button appears in CanvasNodeToolbar when node is configuration node or tool node
- Triggers 'run' event emitted from toolbar
- Executes node with current parameters and input data
- Result displays in OutputPanel
- Disabled during workflow execution or if node has validation errors

#### Implementation
**Entry point:** `CanvasNodeToolbar.vue:60-67 — isExecuteNodeVisible computed property`
**Key files:**
- CanvasNodeToolbar.vue:83-85 — executeNode() method
- CanvasNode.vue:55-71 — emit('run', id) event definition
- Canvas.vue:940 — 'run:node' event emission to parent

#### Dependencies
- Depends on: F-132, F-033
- External: - @n8n/design-system (N8nIconButton)

#### Porting Notes
- Only visible for certain node types; determined by render.type and render.options.configuration flag
- Separate from workflow execution — isolated to single node
- Input data comes from previous execution; no independent input selection UI

---

### F-140: Stop Workflow / Stop Webhook Execution
**Category:** Frontend
**Complexity:** Medium

#### What it does
Buttons to stop currently running workflow or webhook trigger waiting state.

#### Specification
- CanvasStopCurrentExecutionButton — stops active workflow execution
- CanvasStopWaitingForWebhookButton — cancels webhook trigger waiting
- Visible only when execution is active (executing = true) or waiting state active
- Emits 'stopExecution' event to parent
- Disabled in read-only mode

#### Implementation
**Entry point:** `CanvasControlButtons.vue — Container for stop buttons`
**Key files:**
- CanvasStopCurrentExecutionButton.vue — Stop execution UI
- CanvasStopWaitingForWebhookButton.vue — Stop webhook waiting UI
- Canvas.vue:865-875 — onNodeExecutionStop handler

#### Dependencies
- Depends on: F-138, F-014
- External: - @n8n/design-system (N8nButton, icons)

#### Porting Notes
- Buttons conditionally rendered based on execution state flags
- Stop triggers server-side execution termination via WebSocket or API

---

### F-141: Workflow Save with Auto-Save
**Category:** Frontend
**Complexity:** Medium

#### What it does
Saves workflow state periodically and on manual trigger. Handles conflicts, retry logic with exponential backoff, and save state indicators.

#### Specification
- Auto-save triggered on node parameter change (debounced)
- Manual save via Ctrl+S keyboard shortcut
- Tracks save state: Idle, Saving, Saved, Error
- Exponential backoff retry on failure (start delay 1000ms, max 8 retries)
- Conflict modal shown if workflow changed on server
- Save state persisted in useWorkflowSaveStore
- Prevents user navigation if unsaved changes exist

#### Implementation
**Entry point:** `workflowSave.store.ts:15-87 — Pinia store for save state management`
**Key files:**
- workflowSave.store.ts:15-87 — useWorkflowSaveStore with autoSaveState, pendingSave, retry logic
- workflowDocument.store.ts — Document-level workflow state and operations
- Canvas.vue:335-350 — Keyboard shortcut Ctrl+S for manual save

#### Dependencies
- Depends on: F-112
- External: - Pinia (state management)
- Custom retry utility with calculateExponentialBackoff

#### Porting Notes
- Save not triggered by Canvas component — parent app orchestrates save calls
- Conflict detection enables save conflict resolution UI
- Retry logic prevents overwhelming server; exponential backoff configurable
- Auto-save debounce prevents excessive API calls during rapid edits

---

### F-142: Undo / Redo History
**Category:** Frontend
**Complexity:** High

#### What it does
Maintains undo/redo stack for workflow edits. Supports bulk operations, keyboard shortcuts (Ctrl+Z, Ctrl+Y), and history tracking.

#### Specification
- Stack-based implementation: undoStack, redoStack (max 100 items each)
- Bulk command grouping: startRecordingUndo(), stopRecordingUndo()
- Commands support isEqualTo() for deduplication
- Shortcuts: Ctrl+Z (undo), Ctrl+Y (redo)
- Clears redo stack on new action
- Tracks command types: node add, delete, parameter change, connection create, etc.

#### Implementation
**Entry point:** `history.store.ts:9-89 — useHistoryStore Pinia store`
**Key files:**
- history.store.ts:9-89 — Stack management, bulk command handling
- history.ts — Command and BulkCommand model classes
- Canvas.vue:335-350 — Keyboard shortcuts for undo/redo
- useHistoryHelper.ts — Composable for creating history commands

#### Dependencies
- Depends on: F-132
- External: - Pinia (state management)
- STORES constant from @n8n/stores

#### Porting Notes
- History not a feature of Canvas itself; orchestrated by parent app
- Bulk operations group related commands (e.g., move multiple nodes) into single undo unit
- Command deduplication prevents duplicate undo entries for same parameter change
- Max 100 items prevents unbounded memory growth

---

### F-143: Copy / Cut / Paste Nodes
**Category:** Frontend
**Complexity:** Medium

#### What it does
Allows duplicating, moving (cut/paste), and cloning node configurations. Preserves connections when copying groups.

#### Specification
- Copy: Ctrl+C — copies selected nodes to clipboard
- Cut: Ctrl+X — cuts selected nodes, removes from canvas
- Paste: Ctrl+V — pastes nodes at cursor or last known position
- Duplicate: Ctx menu — creates copy within same workflow
- Preserves node connections within copied group
- New nodes offset from originals to avoid overlap
- Supported for node selections (multi-select via Shift+click or Shift+arrow keys)

#### Implementation
**Entry point:** `Canvas.vue:100-102 — 'copy:nodes', 'cut:nodes', 'duplicate:nodes' events`
**Key files:**
- Canvas.vue:338-341 — Keyboard shortcuts: ctrl_c, ctrl_x mapped to copy/cut
- Canvas.vue:100-104 — Event emissions for copy/cut/duplicate
- useClipboard composable (not shown; implied in parent app)

#### Dependencies
- Depends on: F-132, F-153
- External: - Browser Clipboard API (if used by parent)
- @vue-flow/core (selectedNodes)

#### Porting Notes
- Copy/cut/paste logic in Canvas is event emission only; parent app handles clipboard and node creation
- Duplicate differs from copy in that new node created immediately in same workflow
- Connection preservation ensures cloned subgraph maintains internal structure
- Offset logic prevents visual overlap; default offset typically 20-50px

---

### F-144: Keyboard Shortcuts & Navigation
**Category:** Frontend
**Complexity:** Medium

#### What it does
Comprehensive keyboard navigation for canvas: node selection via arrow keys, workflow control via shortcuts, zoom controls, node traversal.

#### Specification
- Arrow keys navigate: Up/Down (sibling nodes), Left (upstream), Right (downstream)
- Shift+Up/Down: select multiple siblings
- Shift+Cmd+Down: select all downstream nodes
- Shift+Cmd+Up: select all upstream nodes
- Space: pan mode toggle (hold to pan)
- Ctrl/Cmd+A: select all nodes
- Ctrl/Cmd+Z: undo
- Ctrl/Cmd+Y: redo
- Ctrl/Cmd+Enter: execute workflow
- Ctrl/Cmd+S: save workflow
- Delete: delete selected node(s)
- Escape: deselect/close panels
- Number 0: reset zoom, 1: fit view, Shift+=/Shift+-: zoom in/out
- Shift+Space: rename selected node (short press)
- Shift+O: open sub-workflow (if subworkflow node selected)

#### Implementation
**Entry point:** `useKeybindings.ts:48-100 — Composable hook for keyboard event binding`
**Key files:**
- useKeybindings.ts:48-100 — Keyboard event listener and shortcut parsing
- Canvas.vue:335-370 — keyMap definition with all shortcuts
- Canvas.vue:273-322 — Node traversal functions: selectLeftNode, selectRightNode, etc.
- Canvas.vue:227-249 — Panning mode toggle via Space key

#### Dependencies
- Depends on: F-132
- External: - @vueuse/core (onKeyDown, onKeyUp, useActiveElement)
- Browser Keyboard API (navigator.keyboard)
- @n8n/composables/useDeviceSupport (detectCtrl/Cmd key)

#### Porting Notes
- useKeybindings composable is generic; keymap defines n8n-specific shortcuts
- Context-aware: shortcuts disabled when editing text (input focused)
- Keyboard layout detection handles international keyboards correctly
- Short-press vs long-press distinguished for Space (rename vs pan)
- Traversal respects node graph edges — only selectable sibling/connected nodes

---

### F-145: Node Search & Creation (Node Creator Panel)
**Category:** Frontend
**Complexity:** High

#### What it does
Searchable panel for browsing and adding nodes to workflow. Supports filtering by category, tags, keywords. Shows node descriptions and quick actions.

#### Specification
- Keyboard shortcut: Cmd+K to open
- Search filters nodes by name, description, tags
- Organized by categories: Triggers, Core, Integration, Transform, etc.
- Supports viewing merged actions and nodes in single view
- Displays node icons, descriptions, ratings
- Quick connect for credentials
- Coachmark for keyboard shortcut discovery (first-time UX)
- Click or drag-to-add node to canvas

#### Implementation
**Entry point:** `NodeCreator.vue:1-40 — Main node creator component`
**Key files:**
- NodeCreator.vue:27-70 — Props, state, panel positioning
- nodeCreator.store.ts — Pinia store with showScrim, actions, viewStacks
- SearchBar.vue — Search input with real-time filtering
- NodesListPanel.vue — Categorized/flat node list rendering
- useNodeCreatorShortcutCoachmark.ts — Coachmark logic for Cmd+K hint

#### Dependencies
- Depends on: F-132
- External: - @n8n/design-system
- Element Plus (modal/panels)
- useNodeTypesStore for node metadata

#### Porting Notes
- Node creator is floating panel; position determined by chat panel width and UI header height
- View stacks enable breadcrumb navigation (Categories → Specific Category → Node Details)
- Merged actions combine node types and workflow actions in single list
- Coachmark visible once per session (onboarding); can be dismissed permanently

---

### F-146: Node Credentials Selection & Management
**Category:** Frontend
**Complexity:** High

#### What it does
UI for selecting, creating, and managing credentials assigned to nodes. Supports OAuth quick-connect, credential search, multi-credential selection for nodes with multiple credential types.

#### Specification
- NodeCredentials component renders credential dropdown(s) for node
- Supports read-only, show-all, and hideIssues display modes
- Credential types: API key, OAuth, basic auth, custom types
- Quick-connect for OAuth credentials (pre-fill via OAuth flow)
- Create new credential inline via button
- Validate credential type matches node requirements
- Show credential usage and permissions
- Warn if credentials are shared (foreign credentials)

#### Implementation
**Entry point:** `NodeCredentials.vue:57-100 — NodeCredentials component`
**Key files:**
- NodeCredentials.vue:57-150 — Main credentials UI component
- credentials.store.ts — useCredentialsStore with credential data
- useNodeCredentialOptions.ts — Credential dropdown options generation
- useCredentialOAuth.ts — OAuth quick-connect logic
- useQuickConnect.ts — Quick connect composable

#### Dependencies
- Depends on: F-135, F-107
- External: - @n8n/design-system (N8nSelect, N8nButton, N8nOption)
- Element Plus (dialogs for new credential creation)
- n8n-workflow (NodeCredentialDescription type validation)
- @n8n/permissions (getResourcePermissions for access control)

#### Porting Notes
- Credential selection triggers 'credentialSelected' event; parent handles node update
- Quick-connect requires OAuth server-side flow; loading state shown during auth
- Foreign credentials (from other projects) shown with warning badge
- Inline credential creation not supported in read-only mode; link to settings instead

---

### F-147: Error Indicators on Nodes
**Category:** Frontend
**Complexity:** Low

#### What it does
Visual indicators (badges, colors) show node validation and execution errors. Clicking error badge opens error details in NDV.

#### Specification
- Execution errors: shown after node execution fails
- Validation errors: shown if node configuration is incomplete/invalid
- Error count badge on node (e.g., "2 errors")
- Tooltip with first error message
- Click error icon to navigate to NDV and show error details
- Errors cleared on next successful execution or manual edit
- Color coded: red (error), orange (warning), yellow (validation)

#### Implementation
**Entry point:** `CanvasNodeStatusIcons.vue — Error icon rendering`
**Key files:**
- CanvasNodeStatusIcons.vue — Renders error icons and badges
- useCanvasNode.ts:52-57 — hasExecutionErrors, hasValidationErrors computeds
- canvas.types.ts:115-119 — CanvasNodeData.issues structure
- CanvasNodeTooltip.vue — Error message tooltip on hover

#### Dependencies
- Depends on: F-010, F-132
- External: - @n8n/design-system (icons)
- Tooltip component

#### Porting Notes
- Issues populated by execution handler and validation logic in parent app
- Errors visible only if issues.visible = true (toggleable)
- Parent app responsible for error message generation and storage
- Error persistence enables users to click and review details in NDV

---

### F-148: Execution History Sidebar
**Category:** Frontend
**Complexity:** Medium

#### What it does
Sidebar showing list of past workflow executions with metadata (status, duration, timestamp). Enables replay and inspection of historical execution data.

#### Specification
- Displays execution list: latest first
- Shows per execution: status badge, node count, duration, timestamp
- Click to load execution data and inspect node results
- Filters: completed, failed, running
- Pagination for large execution lists
- Integrates with NDV: selecting execution updates input/output panels
- Shows pinned data availability per execution

#### Implementation
**Entry point:** `WorkflowHistory.vue — Execution history views (separate from canvas)`
**Key files:**
- workflowHistory.store.ts — useWorkflowHistoryStore with execution list
- executions.store.ts — useExecutionsStore with all executions data
- useNDVStore:111-170 — Tracks selected run/branch/node for result display

#### Dependencies
- Depends on: F-009, F-127
- External: - @n8n/design-system (table, pagination, badges)
- Execution API client

#### Porting Notes
- Execution history not displayed on canvas itself; separate sidebar/panel
- Selecting execution in history auto-loads result data into NDV panels
- Pagination critical for workflows with 100+ executions
- Status filtering reduces list size for large execution counts

---

### F-149: Node Renaming (Inline Rename)
**Category:** Frontend
**Complexity:** Low

#### What it does
Click node name or press Space (short press) to rename node. Changes are saved immediately and reflected across UI.

#### Specification
- Short Space press (not hold) triggers rename mode
- Name input appears inline on node or in tooltip
- Confirm with Enter or blur input
- Cancel with Escape
- Validates uniqueness (name conflicts prevented)
- Emits 'update:node:name' event with new name
- Disabled in read-only mode

#### Implementation
**Entry point:** `Canvas.vue:255-267 — useShortKeyPress for Space rename trigger`
**Key files:**
- Canvas.vue:255-267 — Rename key binding setup
- Canvas.vue:261 — emit('update:node:name', selectedNode.id)
- NodeTitle.vue — Editable node title component (implied)

#### Dependencies
- Depends on: F-132
- External: - @n8n/composables/useShortKeyPress (short vs long press detection)

#### Porting Notes
- Parent app handles name update and validation (uniqueness check)
- Canvas only emits event; doesn't perform validation
- UI feedback (edit mode appearance) handled by parent NDV or node tooltip

---

### F-150: Node Disable/Enable Toggle
**Category:** Frontend
**Complexity:** Low

#### What it does
Toggles node active/disabled state. Disabled nodes are skipped during execution and visually indicated.

#### Specification
- Toggle button in CanvasNodeToolbar (eye icon)
- Keyboard shortcut: none (click only)
- Visual: strikethrough on disabled nodes (CanvasNodeDisabledStrikeThrough)
- Disabled nodes still appear on canvas, connections preserved
- Node remains disabled across saves
- Emits 'update:node:enabled' event

#### Implementation
**Entry point:** `CanvasNodeToolbar.vue:69-71 — isDisableNodeVisible computed`
**Key files:**
- CanvasNodeToolbar.vue:87-89 — onToggleNode() emits toggle event
- CanvasNode.vue:59 — emit('toggle', id)
- CanvasNodeDisabledStrikeThrough.vue — Visual strikethrough effect
- useCanvasNode.ts:45 — isDisabled computed from node data

#### Dependencies
- Depends on: F-132
- External: - @n8n/design-system (N8nIconButton)

#### Porting Notes
- Disabling doesn't remove connections; workflow engine skips execution
- Parent app manages state; canvas reflects via data.disabled property
- Multiple nodes can be disabled; state persisted in workflow document

---

### F-151: Sticky Notes
**Category:** Frontend
**Complexity:** Low

#### What it does
Add and edit sticky note nodes for workflow documentation. Notes are visual elements with customizable colors and text content.

#### Specification
- Create sticky via '+' button or 'Create sticky' context menu option
- Render type: CanvasNodeRenderType.StickyNote
- Content editable inline (click to edit text)
- Resizable: width/height customizable (shown in render options)
- 7 preset colors + custom color picker
- Color selector in node toolbar
- No connections to/from sticky notes
- Text content stored in render.options.content

#### Implementation
**Entry point:** `Canvas.vue:98 — 'create:sticky' event`
**Key files:**
- CanvasNodeStickyNote.vue — Sticky note render component
- CanvasNodeStickyColorSelector.vue — Color picker toolbar item
- CanvasNodeToolbar.vue:79-81 — isStickyNoteChangeColorVisible
- canvas.types.ts:92-100 — CanvasNodeStickyNoteRender type definition

#### Dependencies
- Depends on: F-132
- External: - @n8n/design-system (color picker, form inputs)

#### Porting Notes
- Sticky notes treated as nodes but non-executable
- Resizing updates node dimensions; text content persisted in render.options
- Color changes emit 'update:sticky:color' via node event bus
- No input/output ports or handles

---

### F-152: Zoom & Pan Controls
**Category:** Frontend
**Complexity:** Low

#### What it does
Users can zoom in/out with keyboard shortcuts or buttons, pan canvas, fit all nodes in view, and reset viewport.

#### Specification
- Zoom controls: buttons or keyboard (Shift+=/Shift+-, 0 to reset, 1 to fit)
- Panning: hold Space (or Ctrl/Cmd on desktop) and drag mouse, or middle-mouse drag
- Fit view: fits all nodes in viewport with padding (0.2x, maxZoom default)
- Reset zoom: 1x zoom and center on canvas
- Zoom limits: min 0.5x, max 2x (configurable)
- Viewport state persisted in store; loaded on workflow open
- Mini map shows global overview with draggable viewport rectangle

#### Implementation
**Entry point:** `Canvas.vue:174-203 — useVueFlow hook with zoomIn, zoomOut, fitView, setViewport`
**Key files:**
- Canvas.vue:723-735 — Zoom handlers (onZoomIn, onZoomOut, onResetZoom, onFitView)
- Canvas.vue:244-248 — Space key panning mode toggle
- CanvasControlButtons.vue — UI buttons for zoom/pan controls
- MiniMap component (from @vue-flow/minimap) — Global overview
- useViewportAutoAdjust.ts — Auto-adjust viewport on node addition

#### Dependencies
- Depends on: F-132
- External: - @vue-flow/core (useVueFlow hook with zoom controls)
- @vue-flow/minimap (MiniMap component)
- @vueuse/core (useThrottleFn for pan smoothing)

#### Porting Notes
- Vue Flow handles viewport math; n8n just calls API methods
- Viewport changes emit 'viewport:change' event to parent for tracking
- Auto-adjust on new node insertion prevents adding nodes off-screen
- Panning mode toggle distinguishes selection from panning interactions

---

### F-153: Node Selection (Single & Multi-Select)
**Category:** Frontend
**Complexity:** Medium

#### What it does
Users can select single or multiple nodes for batch operations. Selection indicated by highlight. Multi-select via Shift+click or selection rectangle drag.

#### Specification
- Single-click selects node, emits 'update:node:selected' event
- Shift+click toggles node in/out of selection
- Click canvas (pane) deselects all
- Selection rectangle drag (click-drag on empty pane) selects all nodes within box
- Visual: selected nodes highlighted (border/background color change)
- Keyboard: Ctrl+A selects all nodes
- Selected state tracked in vue-flow nodesSelectionActive and userSelectionRect
- Multiple selected nodes enable batch operations (copy, delete, disable, etc.)

#### Implementation
**Entry point:** `Canvas.vue:989-1020 — onSelectNodes handler`
**Key files:**
- Canvas.vue:989-1020 — onSelectNodes, selected nodes management
- Canvas.vue:175-196 — vue-flow selected nodes API: addSelectedNodes, removeSelectedNodes
- Canvas.vue:343 — Ctrl+A shortcut for select all
- CanvasNode.vue:59-71 — emit('select', id, selected) on click

#### Dependencies
- Depends on: F-132
- External: - @vue-flow/core (getSelectedNodes, addSelectedNodes, removeSelectedNodes, nodesSelectionActive)

#### Porting Notes
- Selection state in vue-flow; Canvas.vue manages selection events
- Range selection (box drag) tracked separately as hasRangeSelection flag
- Parent listens for 'update:node:selected' to trigger NDV open/close
- Batch operations receive selected node IDs, executed one-by-one or grouped

---

### F-154: Sub-Workflow Opening
**Category:** Frontend
**Complexity:** Low

#### What it does
Click on a sub-workflow node to open that workflow in place or new tab. Navigation breadcrumb shows workflow hierarchy.

#### Specification
- Sub-workflow node type: has nested workflow reference
- Click or Shift+O to open sub-workflow
- Emits 'open:sub-workflow' event with node ID
- Parent app navigates to sub-workflow editor
- Breadcrumb shows parent workflow context for back navigation

#### Implementation
**Entry point:** `Canvas.vue:337 — Shift+O shortcut, Canvas.vue:130 — 'open:sub-workflow' event`
**Key files:**
- Canvas.vue:337 — ctrl_shift_o keyboard shortcut
- Canvas.vue:130 — 'open:sub-workflow' event emission

#### Dependencies
- Depends on: F-112
- External: - Router for navigation
- Parent app workflow selector

#### Porting Notes
- Canvas only emits event; parent handles navigation
- Sub-workflow workflow ID passed to open handler
- UI breadcrumb maintained by parent app, not canvas

---

### F-155: Canvas Layout / Auto-Arrange (Tidy Up)
**Category:** Frontend
**Complexity:** Medium

#### What it does
Auto-arrange workflow nodes in hierarchical layout using Dagre algorithm. Triggered via keyboard shortcut, context menu, or button.

#### Specification
- Algorithm: Dagre hierarchical layout (LR direction by default)
- Node spacing: 8x grid units (NODE_X_SPACING, NODE_Y_SPACING)
- Sticky notes remain stationary; regular nodes repositioned
- Selection subset: can tidy only selected nodes or entire workflow
- Result emitted as CanvasLayoutEvent with new positions and bounding box
- Source tracked: 'keyboard-shortcut', 'canvas-button', 'context-menu', 'import-workflow-data'
- Configurable: can disable history tracking (trackHistory: false)

#### Implementation
**Entry point:** `Canvas.vue:119-126 — 'tidy-up' event with CanvasLayoutEvent`
**Key files:**
- useCanvasLayout.ts:52-300 — Layout algorithm using Dagre
- Canvas.vue:700 — onTidyUp handler emits to parent
- canvas.types.ts:190-196 — tidyUp event structure with source tracking

#### Dependencies
- Depends on: F-132
- External: - @dagrejs/dagre (Dagre layout algorithm)

#### Porting Notes
- Layout algorithm respects sticky notes (excluded from layout)
- Special handling for AI nodes: tighter spacing (AI_X_SPACING, AI_Y_SPACING)
- Parent app applies positions; canvas only generates layout
- Source tracking enables telemetry; can be disabled per invocation

---

### F-156: Viewport Auto-Adjustment
**Category:** Frontend
**Complexity:** Low

#### What it does
Automatically pans/zooms viewport to show newly added nodes, ensuring they are visible and not off-screen.

#### Specification
- Triggered when node is created (drag-drop, search add, duplicate)
- Viewport adjusted to include new node position
- Padding applied to avoid edge cases
- Smooth animation (pan/zoom transition)
- Can be disabled for bulk operations

#### Implementation
**Entry point:** `useViewportAutoAdjust.ts — Composable for auto-adjust logic`
**Key files:**
- useViewportAutoAdjust.ts — Viewport auto-adjust on node addition
- Canvas.vue:450 — onDragAndDrop sets newNodeInsertPosition, triggering auto-adjust

#### Dependencies
- Depends on: F-132
- External: - @vue-flow/core (setViewport, fitBounds methods)

#### Porting Notes
- Auto-adjust prevents user disorientation when node appears off-screen
- Can be suppressed during bulk operations (multiple adds) for performance
- Padding prevents node from being at viewport edge

---

### F-157: Context Menu (Right-Click Node Actions)
**Category:** Frontend
**Complexity:** Medium

#### What it does
Right-click on node opens context menu with actions: edit, duplicate, delete, disable, rename, etc.

#### Specification
- Right-click position tracked (mouseX, mouseY)
- Menu items context-aware (read-only disables edit/delete, etc.)
- Actions: delete, duplicate, rename, disable, copy/cut, view docs, etc.
- Supports submenus for complex actions
- Keyboard-triggerable via keyboard shortcut or click menu button
- Closes on click-outside or Escape

#### Implementation
**Entry point:** `Canvas.vue:2-5 — ContextMenu component`
**Key files:**
- ContextMenu.vue — Context menu UI component
- useContextMenu.ts — Composable for menu open/close state
- useContextMenuItems.ts — Generate menu items based on selection
- Canvas.vue:67 — type ContextMenuAction definition

#### Dependencies
- Depends on: F-132, F-153
- External: - @n8n/design-system (Menu, MenuItem components)
- Element Plus (Popover for positioning)

#### Porting Notes
- Menu items generated dynamically based on node type and workflow state
- Context menu emits action events (copy:nodes, delete:node, etc.) to parent
- Position calculated to keep menu within viewport bounds

---

### F-158: Connection Line During Drag (Visual Feedback)
**Category:** Frontend
**Complexity:** Low

#### What it does
While dragging from a node handle to create connection, a live line follows the cursor showing potential connection path.

#### Specification
- Line starts from source handle, follows mouse cursor
- Updates in real-time during drag
- Color/style indicates validity: green (valid), red (invalid)
- Rendered via CanvasConnectionLine component
- SVG-based for performance (vector, not canvas)
- Removed when drag ends (connection created or cancelled)

#### Implementation
**Entry point:** `Canvas.vue:62 — CanvasConnectionLine component`
**Key files:**
- CanvasConnectionLine.vue — Connection line SVG component
- Canvas.vue:1047-1050 — Edge connection line rendering

#### Dependencies
- Depends on: F-134
- External: - SVG rendering in Vue template

#### Porting Notes
- Vue Flow provides connection preview; n8n customizes appearance
- Color changes based on validity without blocking user interaction
- Performance optimized via throttled position updates

---

### F-159: Experimental Embedded NDV (Zoom-Focused Mode)
**Category:** Frontend
**Complexity:** Medium

#### What it does
Alternative NDV mode where node settings appear as embedded panel within canvas when zoomed in on single node. Enables larger, more focused editing experience.

#### Specification
- Experimental feature: toggleable via experimentalNdvStore
- Triggered by special zoom threshold or manual toggle
- NDV embedded in canvas as draggable panel
- Shows larger node configuration with more detail
- Toggle via keyboard shortcut or button (not canonical canvas feature)
- Separate store: experimentalNdvStore with zoom mode state
- Components: ExperimentalNodeDetailsDrawer, ExperimentalEmbeddedNdv*

#### Implementation
**Entry point:** `ExperimentalNodeDetailsDrawer.vue — Embedded NDV component`
**Key files:**
- experimentalNdv.store.ts — useExperimentalNdvStore with isActive, toggleZoomMode
- Canvas.vue:323-333 — onToggleZoomMode handler
- ExperimentalNodeDetailsDrawer.vue — Drawer UI for embedded NDV

#### Dependencies
- Depends on: F-135
- External: - Element Plus (Drawer component)

#### Porting Notes
- Experimental feature; API subject to change
- Separate from main NDV; both can coexist
- Useful for detailed node configuration on large displays

---

### F-160: Node Type Icons & Display Metadata
**Category:** Frontend
**Complexity:** Low

#### What it does
Displays node type icon, name, subtitle, and render metadata (dirtiness, placeholder status) on nodes.

#### Specification
- Icon source: NodeIconSource (URL, icon name, or SVG)
- Name displayed as node label
- Subtitle shows configuration state or connection info
- Dirtiness indicators: parameter updated, incoming connection changed, pinned data updated, upstream dirty
- Placeholder nodes show status (e.g., "Click to configure")
- Community node indicators show verification status
- Icons color-coded by node type

#### Implementation
**Entry point:** `CanvasNode.vue:85-100 — Node data structure`
**Key files:**
- CanvasNodeDefault.vue — Default node render with icon/name/subtitle
- nodeIcon.ts — Icon source resolution logic
- canvas.types.ts:63-80 — CanvasNodeDefaultRender options
- getNodeIconSource utility — Icon URL/name generation

#### Dependencies
- Depends on: F-132
- External: - NodeIcon component from @/app/components
- n8n-workflow (node type metadata)

#### Porting Notes
- Icon resolved from node type metadata; component loads asynchronously
- Dirtiness state set by parent app based on workflow changes
- Placeholder state used for nodes awaiting configuration

---
## Porting Checklist

- [ ] 1. **Implement DAG execution engine** — port `WorkflowExecute` (F-001 through F-021): node scheduling stack, multi-input synchronization, retry, timeout, cancellation, and lifecycle hooks before any other feature.
- [ ] 2. **Port database layer** — set up TypeORM equivalent with all entities (F-103 to F-131): ExecutionEntity, WorkflowEntity, CredentialsEntity, User, Project, Webhook, Settings, Variables, Tags, AuthIdentity. Define migration DSL first (F-105/F-106).
- [ ] 3. **Implement trigger system** — webhook registration, live execution, path routing, cron/poll activation, multi-instance coordination (F-022 to F-038). This requires F-001 and F-103 complete.
- [ ] 4. **Build REST API scaffolding** — controller registry with decorator-based routing, Zod validation, response envelopes, rate limiting, pagination (F-056 to F-079). All API features depend on this.
- [ ] 5. **Implement JWT authentication** — cookie-based JWT issuance/validation (F-080), email/password auth (F-081), MFA (F-086), logout/invalidation (F-089), browser ID hijack prevention (F-098).
- [ ] 6. **Add RBAC and permission scopes** — role model (F-090), global scopes (F-091), credential sharing (F-092), workflow sharing (F-093), project membership (F-094), license gates (F-095).
- [ ] 7. **Implement real-time push** — WebSocket server with HTTP upgrade (F-073, F-066), SSE fallback (F-067), push routing/serialization (F-069 to F-071), auth validation (F-072). Required for UI execution feedback.
- [ ] 8. **Implement queue/scaling mode** — Redis connection (F-053), Bull queue setup (F-040), worker process (F-042 to F-044), job serialization (F-041), concurrency control (F-045, F-046), graceful shutdown (F-048), queue recovery (F-049).
- [ ] 9. **Implement multi-main PubSub** — Redis PubSub channels for multi-instance coordination (F-054), leader election for webhook deduplication (F-030), trigger activation broadcasting (F-031).
- [ ] 10. **Implement credential encryption** — AES-256 encryption layer (F-124) using a master encryption key stored in settings. Implement redaction (F-126) and OAuth token preservation on update (F-125).
- [ ] 11. **Port SSO providers** — LDAP (F-083), SAML 2.0 (F-084), OIDC (F-085), JIT provisioning (F-100), auth method switching (F-099). Each requires the JWT base (F-080) and AuthIdentity entity (F-116).
- [ ] 12. **Build Vue canvas frontend** — Vue Flow graph renderer (F-132), node drag-and-drop (F-133), edge creation (F-134), NDV panel (F-135), expression editor (F-136), execution results overlay (F-137).
- [ ] 13. **Wire frontend run/stop controls** — run button with trigger selection (F-138), manual node execution (F-139), stop execution (F-140), push event consumer to update node status (F-069).
- [ ] 14. **Implement frontend state management** — undo/redo history (F-142), auto-save (F-141), clipboard copy/paste (F-143), keyboard shortcuts (F-144), node creator panel (F-145).
- [ ] 15. **Add expression editor** — CodeMirror 6 with n8n expression autocompletion (F-136). Requires the expression evaluation engine (F-008) to be exposed via an API or shared module.
- [ ] 16. **Implement execution history UI** — sidebar listing past executions (F-148) with status, timing, and re-run capability. Depends on ExecutionRepository filters (F-127).
- [ ] 17. **Add metric export** — Prometheus endpoint via prom-client (F-050). Queue metrics require Bull event listeners. Only run on main instance.
- [ ] 18. **Implement task runner sandboxing** — external child-process task runner (F-055) for Code nodes. Python sidecar optional. Interface via IPC message passing.
- [ ] 19. **Add partial execution** — DirectedGraph sub-DAG extraction (F-012) for "run from here" and "run to here" UI features. Requires full DAG engine.
- [ ] 20. **Validate license gating** — License service (F-095) must be initialized before any EE-gated feature is accessed. API key scopes (F-101) and worker status monitoring (F-052) are license-gated.
