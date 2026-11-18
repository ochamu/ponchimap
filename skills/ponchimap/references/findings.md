# Findings files

A findings file is the JSON you write for `ponchimap import`. One file reports one read of one source. `import` merges every file in `.ponchimap/findings/` into `.ponchimap/model.json`, and `render` draws the model.

## Files and sources

A source is one thing you ask, asked the same way every time. Name it by route, provider, and environment:

| What you read | `source` | `route` |
|---|---|---|
| The repository: config, IaC, CI, code, manifests | `repo` | `repository` |
| Documents: README, `docs/`, issues, wikis, MCP documents | `docs` | `document` |
| A provider's CLI, for one environment | `cli-<provider>-<environment>`, such as `cli-cloudflare-production` | `cli` |
| An MCP server, for one environment | `mcp-<server>-<environment>` | `mcp` |

When one environment spans two accounts or projects of one provider, add the account or project to the name.

- Write each source to `.ponchimap/findings/<source>.json`. Keep one file per source.
- On every run, rewrite the whole file: a new `acquisition.id`, a new `checked_at`, the same `source`. Do not keep dated copies. A file stays in the model until a newer read of its source with `complete: true` replaces it, and until then `import` fails with `import.no_evidence` when a candidate in it names a node that is gone.
- Never change a file and keep its `acquisition.id`. `import` refuses it with `import.acquisition_changed`.
- Give each environment its own source. Under one source name, the newer complete file drops what the other environment's file said.
- Deleting a file does not remove what it said from the model. To drop a source, rewrite its file with `complete: true` and no candidates.

## Top level

| Key | Required | Meaning |
|---|---|---|
| `schema_version` | yes | `"0.1"` |
| `acquisition` | yes | The read this file reports |
| `nodes`, `edges`, `coverage` | no | Candidates. A missing list is an empty list |
| `diagnostics` | no | Notes for the user: `{"message": "...", "code": "...", "target_ids": [...]}` |

## `acquisition`

| Key | Meaning |
|---|---|
| `id` | New for every read: `<source>-<yyyymmdd>-<hhmmss>` in UTC, such as `repo-20261117-090000` |
| `source` | The source name from the table above |
| `route` | `repository`, `document`, `cli`, or `mcp` |
| `operation` | What you ran or read, such as `wrangler d1 list --json` or `read docs/operations.md`. For a provider you did not query, `not run:` and the reason. No secrets, emails, or tokens |
| `target` | What you read: any of `repository`, `provider`, `account_id`, `project_id`, `environment`, `service`, `filter`. Never `target_ids` |
| `checked_at` | When the read finished, such as `2026-11-17T09:00:00Z`. Run `date -u +%Y-%m-%dT%H:%M:%SZ` once when the read of a source ends, and use that time for `acquisition.id` and for every `checked_at` in the file. Do not invent or round a time |
| `complete` | `true` when this file states everything the source says now. `false` when any part of the read failed, was denied, or was skipped |

`complete: true` lets `import` drop what an older read of the same source said and this file does not repeat. `complete: false` keeps the older evidence that this file does not restate.

## IDs

Every `id` you write (candidates, evidence, coverage, acquisition) matches `^[a-z][a-z0-9-]*$`: lowercase letters, digits, and hyphens, starting with a letter. Candidate IDs are names inside one file, and `import` gives model IDs such as `n-2sde7vfm`. All IDs in one file share one namespace, so give evidence and coverage IDs of their own, such as `api-declared`.

## Nodes

```json
{"id": "api", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.workers",
 "identity": {"account_id": "...", "native_id": "photo-api"},
 "attributes": {"role": "API for uploads and orders", "service_name": "Cloudflare Workers", "category": "compute"},
 "evidence": [...]}
```

| Key | Meaning |
|---|---|
| `kind` | `resource`: something the user manages (a Worker, a database, a bucket, a SaaS project of their own). `service`: an external API in general, such as Stripe or Resend, with no managed resource identified. A SaaS tenant the user set up, such as an Algolia application or a Sentry project, is a `resource` when you know its identifier, and a `service` otherwise. A project is either one node or the owner of the nodes inside it, never both: when you draw the products inside a project as nodes, such as its database and its sign-in, the project is their `scope.owner` |
| `provider` | Who runs it: `aws`, `gcp`, `azure`, `cloudflare`, `firebase`, `supabase`, `vercel`, or `github`. For any other vendor, its name in lowercase as its own domain spells it, such as `stripe` or `sendgrid` (Twilio SendGrid, sendgrid.com). The figure names account boundaries after it |
| `service` | The product's key, such as `cloudflare.d1` or `aws.lambda`. See "Service keys, names, and kinds" below |
| `identity` | How `import` tells resources apart. See the next section |
| `environment` | Only when the node belongs to an environment other than `acquisition.target.environment` |
| `attributes.role` | What it does for this app, in a few words in the user's language, or in English when the bundled font lacks its letters (see the Skill). The figure prints it under the name |
| `attributes.service_name` | On every node: the product's name as the vendor's own documentation writes it, with the vendor's prefix when the vendor uses one, such as `Cloudflare D1`, `AWS Lambda`, `Twilio SendGrid`, or `Cloud Run`. The card and the report name the node by it, and show the `service` key when it is missing. It is not the resource's own name, such as `make-thumbnails`. Write the same name on every node with that key, on one line. `import` refuses an empty name, a name of only spaces, and a line break |
| `attributes.category` | On every node: one of the values in the table below. Pick the value that says what the service does for this app. When two fit, pick by how this app uses it, not by how the product is sold. An external API with no closer row is `api`. When nothing fits, not even `api`, leave it out, and the card gets the generic grey icon. `import` refuses any other value |
| `evidence` | At least one item, except for a candidate that only names an existing node (see Coverage) |

### Service keys, names, and kinds

The CLI knows no service. Each card shows the name and the icon that its node's `attributes.service_name` and `attributes.category` give, so write both on every node, whatever the key. Leave the category out only when no kind fits, not even `api`.

- Write the key as `<provider>.<product>` in lowercase, with hyphens between words, such as `aws.lambda` or `cloudflare.durable-objects`. When the product's name is just the provider's name, such as Stripe or Resend, the key is the provider alone: `stripe`, `resend`.
- What the user runs, such as a browser or a mobile app, has the provider `client`: `client.browser`, `client.ios`.
- `import` matches nodes by the key. Decide it once, in the identity table, and copy it to every file. On a repeat request, copy it from the existing findings files.

| `attributes.category` | What the service does for the app | Example `service` key |
|---|---|---|
| `api` | An external API the app calls that no other row fits better, such as search or a bot check | `anthropic.api`, `algolia` |
| `auth` | Sign-in and user accounts | `aws.cognito` |
| `client` | What the user runs, such as a browser or a mobile app | `client.ios` |
| `compute` | Runs the app's code | `aws.lambda` |
| `database` | Keeps records in tables and answers queries | `aws.rds` |
| `hosting` | Serves the site or its pages | `netlify` |
| `keyvalue` | Keeps values by key, such as a cache or a store of secrets | `upstash.redis` |
| `mail` | Sends email | `sendgrid` |
| `message` | Sends push notifications, SMS, or chat messages to users | `onesignal` |
| `monitor` | Collects errors, logs, or metrics | `datadog` |
| `network` | Routes traffic, such as DNS | `aws.route53` |
| `payment` | Takes payments | `paypal` |
| `pipeline` | Builds and deploys, or runs scheduled jobs such as a backup | `circleci` |
| `queue` | Holds work for another part of the app to do later | `aws.sqs` |
| `storage` | Keeps files and objects | `aws.s3` |

```json
{"id": "thumbs", "kind": "resource", "provider": "aws", "service": "aws.lambda",
 "identity": {"native_id": "make-thumbnails"},
 "attributes": {"role": "Makes thumbnails of uploads", "service_name": "AWS Lambda", "category": "compute"},
 "evidence": [...]}
```

## Identity

`import` matches a candidate to a node by `kind`, `provider`, `service`, and the identity keys `native_id`, `logical_name`, `account_id`, and `project_id`. Write them as non-empty strings, and quote digits. Other keys are kept but not matched.

- `native_id`: the identifier at the provider (a Worker name, a D1 `database_id`, a bucket name). Prefer it. Use the shortest identifier that is unique in the account and that the provider's console and CLI show, such as an SQS queue's name rather than its URL or ARN. When the configuration gives both a name and an ID that the provider assigned, `native_id` is the ID and `logical_name` is the name. When the same name is used in another region of the account, the name is not unique: use the provider's full identifier that includes the region, such as an AWS ARN. Give the region as `region` evidence too.
- `logical_name`: the name the configuration gives the resource, such as a D1 `database_name`. Write it whenever the configuration gives one, next to `native_id` too: the report lists it. A file that knows only `native_id`, such as a query result, still matches the same node, when another file writes that `native_id` together with the `logical_name`.
- `account_id`, `project_id`: the owner. Leave them out when not known, and never guess. Take one from a signed-in CLI only after you have checked that its account is the target. For a SaaS tenant, `account_id` is the vendor account or organization that holds it.
- A `resource` needs `native_id` or `logical_name`. A `service` usually has no identity.
- A product that an account or a project has exactly one of, such as its sign-in service: `native_id` is the account or project ID.
- A default instance that the configuration does not name, such as a project's default database or bucket: `native_id` is the name the provider's CLI shows for it. Until you know that name, write `logical_name: "default"`. When a query shows the name, add it as `native_id` to your identity table, and write it next to `logical_name: "default"` in every file, the query's file too. A file with the `native_id` alone makes a second node.

The rule:

1. Before writing any file, decide each resource's identity once, in a table, by the rules above.
2. Copy that identity exactly into every file that mentions the resource.
3. On a repeat request, read the identities from the existing findings files and copy them.
4. When you learn a key later (the account, or the `native_id`), add it and keep every key you wrote before. Never swap or drop a key. Decide the owner key once: an identity that has `project_id` does not get `account_id` later, and the reverse.

What breaks it, as measured with `import`:

| Two files describe one D1 database with | Result |
|---|---|
| `logical_name` in one, `native_id` in the other | Two nodes |
| `logical_name` + `native_id` in one, `logical_name` + `account_id` in the other | Two nodes |
| `native_id` in both, `account_id` in only one | One node |
| A later run replaces `logical_name` with `native_id` | The node is removed and comes back with a new ID, and view labels on the old ID are lost |
| A later run adds `native_id` or `account_id` and keeps the old keys | Same node, same ID |
| A later run adds `account_id` to an identity that has `native_id` and `project_id` | The node is removed and comes back with a new ID |

## Edges

```json
{"id": "api-binds-photos", "from": "api", "to": "photos", "relation": "configuration", "slot": "PHOTOS",
 "attributes": {"purpose": "Store originals"}, "evidence": [...]}
```

| Key | Meaning |
|---|---|
| `from`, `to` | Candidate node IDs in the same file |
| `relation` | `configuration`: a connection in settings, such as a binding. `implementation`: a call in code. `observation`: traffic seen in logs. `dependency`: one needs the other to start or deploy |
| `slot` | The setting that holds a configuration connection, such as the binding name `DB`, the name of an injected variable, or the path pattern of a rewrite |
| `environment` | Only when it differs from `acquisition.target.environment` |
| `attributes.purpose` | What the relation is for, in a few words in the user's language, or in English when the bundled font lacks its letters (see the Skill). The figure prints it on the line |

Point the edge the way the call or the setting goes: the Worker binds the database, and the web app calls the API. A trigger points from the source of the event to what it starts: the queue to its consumer, the bucket to the function that its uploads start. A trigger is a `configuration` edge, with `declared` evidence that cites the file where it is set, even when that file is code.

- A setting and a call between the same two nodes are two edges.
- A rewrite or a proxy rule that sends requests on to another resource is a connection: a `configuration` edge.
- Decide the `slot` of each `configuration` edge once, and write the same value in every file. An edge with a slot in one file and none in another becomes two edges.
- A connection that the configuration declares and no code uses is still a `configuration` edge. Mention in your answer that no code uses it.
- Routes, custom domains, and schedules say how a resource is reached or run. Write them in that resource's `role`. Draw a node for one only when a source names it as a resource of its own, as the domain in the last example below, or when the user asked to see it.

## Evidence

```json
{"id": "api-declared", "kind": "declared",
 "supports": [{"key": "existence", "value": true}],
 "ref": {"type": "file", "path": "wrangler.toml", "start_line": 1, "end_line": 5},
 "checked_at": "2026-11-17T09:00:00Z"}
```

| `kind` | Use it for |
|---|---|
| `declared` | Settings in the repository: deploy config, IaC, CI, and settings that code declares, such as a function's region or trigger |
| `deployed` | What a query of the real environment returned |
| `code` | A concrete call in code, where the caller, the target, and what it does are clear |
| `observed` | Traffic or logs that the user gave you. Add `valid_time` for the period |
| `stated` | A document or a person said it |
| `hint` | A clue: an SDK import with no call you can trace, an environment variable name |

An SDK import alone is a `hint`, never `code`. A file that declares something is `declared`, never `deployed`. The route does not change the kind: a CLI query that returns a setting is `deployed`.

| Support key | Value | Meaning |
|---|---|---|
| `existence` | `true` or `false` | The node exists. Write `false` only when a complete, targeted query did not find it, with a `complete` coverage record for that scope in the same file |
| `scope.owner` | `{"kind": "account", "id": "..."}`, with `kind` one of `account`, `project`, `organization`, `team` | The owner. Add it to `declared` or `deployed` evidence whenever the source states the account or project: there it draws the account or project box. The account or the project that holds the nodes you draw is never a node itself |
| `region` | string | The region |
| `scope.network` | string | The one network the node is in, such as a VPC ID. Drawn only with a `region` |
| `exposure` | string, such as `public` | How far it can be reached |
| `connection.configured` | `true` or `false` | A configuration edge is set |
| `connection.target` | a candidate node ID | The node a configured connection points at |
| `call.implemented` | `true` | Code makes the call |
| `traffic.observed` | `true` | Traffic was seen during `valid_time` |
| `lifecycle.requires` | `true` | A dependency edge |
| `role` | string | What a node does, as a source states it |

`ref.type` is `file` (a repository file, with `start_line` and `end_line` when you can), `document` (a document, or its URL), `query` (the command or API call you ran, without secrets), or `log`. `checked_at` is when you read it. `valid_time` is when the fact held, for observations and old documents.

## How the figure shows evidence

| Element | Solid | Dashed |
|---|---|---|
| `resource` node | `declared` or `deployed` with `existence: true` | anything else, such as `code`, `stated`, or `hint` alone |
| `service` node | `code` with `call.implemented: true`, or `observed` with `traffic.observed: true` | anything else |
| `configuration` edge | `declared` or `deployed` with `connection.configured: true` | only `stated` or `hint` |
| `implementation` edge | `code` with `call.implemented: true` | only `stated` or `hint` |

| Mark | When |
|---|---|
| 未照合 (unverified) on a node | A `resource` has `declared` existence and no `deployed` existence, or a later query naming it did not finish |
| 未照合 on a configuration edge | A `declared` connection has no `deployed` connection to the same target |
| （説明・未照合） or （手がかり） on a line | The edge rests on `stated` or `hint` only |
| 不一致 (mismatch) | Two sources give different values for the same item, such as a region or a binding target |

`service` nodes never get 未照合. The report lists every dashed element under its unverified points.

## Coverage

Coverage says how far a query got. Write one record per query and target.

```json
{"id": "photos-denied",
 "scope": {"provider": "cloudflare", "account_id": "...", "environment": "production",
           "service": "cloudflare.r2", "target_ids": ["photos"], "filter": {"fields": ["existence"]}},
 "status": "denied", "reason": "...", "checked_at": "2026-11-17T09:10:00Z"}
```

| `status` | When |
|---|---|
| `complete` | Everything in the scope was read |
| `partial` | Part is missing, such as a list that stopped at a page |
| `denied` | No permission |
| `failed` | The read did not finish because of the network, a timeout, or an error |
| `unsupported` | No read-only way to read it, or no way that you could confirm to only read and to keep secret values out of your context |
| `not_attempted` | Not tried: the CLI is missing or signed in elsewhere, or the user said skip |

- `target_ids` names the candidates the query was about. To name a node that another file describes, add a candidate with the same `kind`, `provider`, `service`, and the exact same `identity`, and `"evidence": []`. If the identity differs, `import` fails with `import.no_evidence`.
- `filter.fields` lists the support keys the query read, such as `["existence"]`. Without it, a `complete` record also covers items you did not read, such as the node's bindings.
- To check a `configuration` edge, query the deployed settings of the resource that holds the setting. The coverage record is about the node the edge starts from: `target_ids` names that node, `scope.service` is that node's key, `filter.slot` is the slot, and `filter.fields` is `["connection.configured", "connection.target"]`. That a resource exists does not show that its deployed settings match the file. Write what the query showed:
  - The same target: the edge again with the same `from`, `to`, and `slot`, and `deployed` evidence with `connection.configured: true` and `connection.target`.
  - Another target: an edge with the same `from` and `slot` to the node that the output names. The figure marks the mismatch.
  - No such setting: the edge again with `deployed` evidence `connection.configured: false`, and a `complete` coverage record.
  - Write `connection.target` only when the output shows the target's identifier. When a variable holds the target, do not read its value: compare it inside the pipeline so that only true or false prints, such as `jq '.env.DB_HOST | test("db\\.example\\.com")'`. When you saw only that the variable is set, write no `deployed` evidence on the edge, and record `unsupported` coverage for that slot.
- `reason` is printed in the report as written. Write it in the user's language, and say what would fix it: the permission needed, or the account to sign in to.
- A list that returned nothing proves absence only with a `complete` record for that exact scope.

## Unconfirmed clues

- A package listed in a manifest that no code imports: write no node, and tell the user in your answer.
- An import, a config entry, or an environment variable name with no call you can trace: a `hint` node with `existence: true`, and an `implementation` edge from the component that holds the clue, with `hint` evidence `call.implemented: true` and a `purpose` that says what the clue is. Both cite the file of the clue. The figure draws both dashed, the line ends in （手がかり）, and the report lists them as unverified.
- A concrete call: `code`.

## Examples

The four files below describe one made-up app. They pass `import --environment production` together. They show the form of the records. The commands in `operation` and `ref.path` are not a command reference: find and check every command you run as the Skill's rules say.

The repository read:

```json findings
{
  "schema_version": "0.1",
  "acquisition": {
    "id": "repo-20261117-090000",
    "source": "repo",
    "route": "repository",
    "operation": "read firebase.json, .firebaserc, wrangler.toml, package.json, web/src, api/src",
    "target": {"repository": ".", "environment": "production"},
    "checked_at": "2026-11-17T09:00:00Z",
    "complete": true
  },
  "nodes": [
    {
      "id": "web", "kind": "resource", "provider": "firebase", "service": "firebase.hosting",
      "identity": {"project_id": "photo-prod-example", "logical_name": "default"},
      "attributes": {"role": "Serves the photo sharing web app", "service_name": "Firebase Hosting", "category": "hosting"},
      "evidence": [{
        "id": "web-declared", "kind": "declared",
        "supports": [
          {"key": "existence", "value": true},
          {"key": "scope.owner", "value": {"kind": "project", "id": "photo-prod-example"}}
        ],
        "ref": {"type": "file", "path": "firebase.json", "start_line": 2, "end_line": 9},
        "checked_at": "2026-11-17T09:00:00Z"
      }]
    },
    {
      "id": "api", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.workers",
      "identity": {"account_id": "aaaa1111bbbb2222cccc3333dddd4444", "native_id": "photo-api"},
      "attributes": {"role": "API for uploads, albums and print orders", "service_name": "Cloudflare Workers", "category": "compute"},
      "evidence": [{
        "id": "api-declared", "kind": "declared",
        "supports": [
          {"key": "existence", "value": true},
          {"key": "scope.owner", "value": {"kind": "account", "id": "aaaa1111bbbb2222cccc3333dddd4444"}}
        ],
        "ref": {"type": "file", "path": "wrangler.toml", "start_line": 1, "end_line": 5},
        "checked_at": "2026-11-17T09:00:00Z"
      }]
    },
    {
      "id": "photos", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.r2",
      "identity": {"account_id": "aaaa1111bbbb2222cccc3333dddd4444", "native_id": "photo-originals"},
      "attributes": {"role": "Keeps the uploaded originals", "service_name": "Cloudflare R2", "category": "storage"},
      "evidence": [{
        "id": "photos-declared", "kind": "declared",
        "supports": [
          {"key": "existence", "value": true},
          {"key": "scope.owner", "value": {"kind": "account", "id": "aaaa1111bbbb2222cccc3333dddd4444"}}
        ],
        "ref": {"type": "file", "path": "wrangler.toml", "start_line": 18, "end_line": 20},
        "checked_at": "2026-11-17T09:00:00Z"
      }]
    },
    {
      "id": "stripe", "kind": "service", "provider": "stripe", "service": "stripe",
      "attributes": {"role": "Takes payment for print orders", "service_name": "Stripe", "category": "payment"},
      "evidence": [{
        "id": "stripe-code", "kind": "code",
        "supports": [{"key": "call.implemented", "value": true}],
        "ref": {"type": "file", "path": "api/src/orders.ts", "start_line": 31, "end_line": 48},
        "checked_at": "2026-11-17T09:00:00Z"
      }]
    }
  ],
  "edges": [
    {
      "id": "web-calls-api", "from": "web", "to": "api", "relation": "implementation",
      "attributes": {"purpose": "Upload and list photos"},
      "evidence": [{
        "id": "web-calls-api-code", "kind": "code",
        "supports": [{"key": "call.implemented", "value": true}],
        "ref": {"type": "file", "path": "web/src/client.ts", "start_line": 8, "end_line": 40},
        "checked_at": "2026-11-17T09:00:00Z"
      }]
    },
    {
      "id": "api-binds-photos", "from": "api", "to": "photos", "relation": "configuration", "slot": "PHOTOS",
      "attributes": {"purpose": "Store originals"},
      "evidence": [{
        "id": "api-binds-photos-declared", "kind": "declared",
        "supports": [
          {"key": "connection.configured", "value": true},
          {"key": "connection.target", "value": "photos"}
        ],
        "ref": {"type": "file", "path": "wrangler.toml", "start_line": 18, "end_line": 20},
        "checked_at": "2026-11-17T09:00:00Z"
      }]
    },
    {
      "id": "api-calls-stripe", "from": "api", "to": "stripe", "relation": "implementation",
      "attributes": {"purpose": "Charge for a print order"},
      "evidence": [{
        "id": "api-calls-stripe-code", "kind": "code",
        "supports": [{"key": "call.implemented", "value": true}],
        "ref": {"type": "file", "path": "api/src/orders.ts", "start_line": 31, "end_line": 48},
        "checked_at": "2026-11-17T09:00:00Z"
      }]
    }
  ]
}
```

A CLI query that found the Worker and was refused the bucket list. The bucket candidate has no evidence and copies its identity from `repo.json`, so the denied record names that node:

```json findings
{
  "schema_version": "0.1",
  "acquisition": {
    "id": "cli-cloudflare-production-20261117-091000",
    "source": "cli-cloudflare-production",
    "route": "cli",
    "operation": "wrangler whoami; wrangler deployments status --name photo-api; wrangler r2 bucket list",
    "target": {"provider": "cloudflare", "account_id": "aaaa1111bbbb2222cccc3333dddd4444", "environment": "production"},
    "checked_at": "2026-11-17T09:10:00Z",
    "complete": false
  },
  "nodes": [
    {
      "id": "api", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.workers",
      "identity": {"account_id": "aaaa1111bbbb2222cccc3333dddd4444", "native_id": "photo-api"},
      "attributes": {"service_name": "Cloudflare Workers", "category": "compute"},
      "evidence": [{
        "id": "api-deployed", "kind": "deployed",
        "supports": [
          {"key": "existence", "value": true},
          {"key": "scope.owner", "value": {"kind": "account", "id": "aaaa1111bbbb2222cccc3333dddd4444"}}
        ],
        "ref": {"type": "query", "path": "wrangler deployments status --name photo-api"},
        "checked_at": "2026-11-17T09:10:00Z"
      }]
    },
    {
      "id": "photos", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.r2",
      "identity": {"account_id": "aaaa1111bbbb2222cccc3333dddd4444", "native_id": "photo-originals"},
      "attributes": {"service_name": "Cloudflare R2", "category": "storage"},
      "evidence": []
    }
  ],
  "coverage": [
    {
      "id": "api-checked",
      "scope": {
        "provider": "cloudflare", "account_id": "aaaa1111bbbb2222cccc3333dddd4444", "environment": "production",
        "service": "cloudflare.workers", "target_ids": ["api"], "filter": {"fields": ["existence"]}
      },
      "status": "complete",
      "reason": "photo-api has an active deployment",
      "checked_at": "2026-11-17T09:10:00Z"
    },
    {
      "id": "photos-denied",
      "scope": {
        "provider": "cloudflare", "account_id": "aaaa1111bbbb2222cccc3333dddd4444", "environment": "production",
        "service": "cloudflare.r2", "target_ids": ["photos"], "filter": {"fields": ["existence"]}
      },
      "status": "denied",
      "reason": "wrangler r2 bucket list was refused: the signed-in token cannot read R2. Sign in with a token that has R2 read access and ask again",
      "checked_at": "2026-11-17T09:10:00Z"
    }
  ]
}
```

A query you did not run. The Firebase CLI was signed in to another account, and the user chose to skip it:

```json findings
{
  "schema_version": "0.1",
  "acquisition": {
    "id": "cli-firebase-production-20261117-091500",
    "source": "cli-firebase-production",
    "route": "cli",
    "operation": "firebase login:list",
    "target": {"provider": "firebase", "project_id": "photo-prod-example", "environment": "production"},
    "checked_at": "2026-11-17T09:15:00Z",
    "complete": false
  },
  "nodes": [
    {
      "id": "web", "kind": "resource", "provider": "firebase", "service": "firebase.hosting",
      "identity": {"project_id": "photo-prod-example", "logical_name": "default"},
      "attributes": {"service_name": "Firebase Hosting", "category": "hosting"},
      "evidence": []
    }
  ],
  "coverage": [
    {
      "id": "web-skipped",
      "scope": {
        "provider": "firebase", "project_id": "photo-prod-example", "environment": "production",
        "service": "firebase.hosting", "target_ids": ["web"]
      },
      "status": "not_attempted",
      "reason": "The Firebase CLI is signed in to an account that does not list photo-prod-example; the user chose to skip this query",
      "checked_at": "2026-11-17T09:15:00Z"
    }
  ]
}
```

A document read. The custom domain is set up in the dashboard and written only in `docs/operations.md`, so the node and the edge are `stated` and draw dashed:

```json findings
{
  "schema_version": "0.1",
  "acquisition": {
    "id": "docs-20261117-090500",
    "source": "docs",
    "route": "document",
    "operation": "read README.md and docs/operations.md",
    "target": {"repository": "."},
    "checked_at": "2026-11-17T09:05:00Z",
    "complete": true
  },
  "nodes": [
    {
      "id": "web", "kind": "resource", "provider": "firebase", "service": "firebase.hosting",
      "identity": {"project_id": "photo-prod-example", "logical_name": "default"},
      "attributes": {"service_name": "Firebase Hosting", "category": "hosting"},
      "evidence": []
    },
    {
      "id": "domain", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.dns",
      "identity": {"account_id": "aaaa1111bbbb2222cccc3333dddd4444", "logical_name": "photos.example.com"},
      "attributes": {"role": "Public name of the web app", "service_name": "Cloudflare DNS", "category": "network"},
      "evidence": [{
        "id": "domain-stated", "kind": "stated",
        "supports": [{"key": "existence", "value": true}],
        "ref": {"type": "document", "path": "docs/operations.md", "start_line": 4, "end_line": 9},
        "checked_at": "2026-11-17T09:05:00Z"
      }]
    }
  ],
  "edges": [
    {
      "id": "domain-points-to-web", "from": "domain", "to": "web", "relation": "configuration",
      "attributes": {"purpose": "Custom domain, set up in the dashboard"},
      "evidence": [{
        "id": "domain-points-to-web-stated", "kind": "stated",
        "supports": [{"key": "connection.configured", "value": true}],
        "ref": {"type": "document", "path": "docs/operations.md", "start_line": 4, "end_line": 9},
        "checked_at": "2026-11-17T09:05:00Z"
      }]
    }
  ]
}
```
