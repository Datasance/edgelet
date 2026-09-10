# Controller handoff — registries and models

This document is for the **Pot / Datasance Controller** team. It describes JSON shapes and sync flags Edgelet is ready to consume for registry type/TLS fields and model artifacts.

**On the agent today:** local deploy (`kind: Registry`, `kind: Model`) and EdgeletAPI `/v1/models*` are implemented. Controller CRUD, `getChanges` model sync, and agent status model fields are **not** implemented — store rows for controller models exist as a stub only.

Operator YAML: [manifest-reference.md](manifest-reference.md) · [models.md](models.md).

---

## Registry object (Pot JSON)

Extend the existing registry payload. **`type`** defaults to `"oci"` when omitted.

```json
{
  "id": 5,
  "url": "https://huggingface.co",
  "isPublic": false,
  "userName": "",
  "password": "hf_…",
  "userEmail": "",
  "type": "hf",
  "ca": "",
  "insecure": false
}
```

| Field | Type | Notes |
|-------|------|--------|
| `type` | string | `"oci"` (default) or `"hf"` |
| `ca` | string | Optional base64 PEM CA bundle |
| `insecure` | boolean | Default `false`; `true` allows `http://` and skips TLS verify |

Existing **`registries: true`** on `getChanges` should reload these extended rows (including `type`) once the controller sends them. Image pull on the agent still requires `type: oci`.

---

## Model object (Pot JSON)

Design-only until the controller adds CRUD. Suggested shape:

```json
{
  "id": 12,
  "name": "llama-2-7b-q2k",
  "repo": "second-state/Llama-2-7B-Chat-GGUF",
  "revision": "064fe43ea8c1e1f93477ef4a170bdc2b244ef02c",
  "registryId": 5,
  "files": ["llama-2-7b-chat.Q5_K_M.gguf"],
  "format": "gguf"
}
```

| Field | Type | Notes |
|-------|------|--------|
| `id` | integer | Controller primary key |
| `name` | string | DNS-1123; matches Edgelet `metadata.name` |
| `repo` | string | Upstream path **without** host |
| `revision` | string | Pin; empty means `latest` (OCI) or `main` (HF) |
| `registryId` | integer | Registry row id; type must match the pull adapter |
| `files` | string[] | HF only; ignored for OCI |
| `format` | string | Optional hint: `gguf`, `safetensors`, `onnx`, `pytorch`, `tensorrt`, `unknown` |

---

## getChanges

| Flag | Status on Edgelet | Expected controller behavior |
|------|-------------------|------------------------------|
| `registries` | Existing | Reload registry rows including `type`, `ca`, `insecure` |
| `models` | **Not implemented** | New flag **`models: true`** — push/replace controller model list |

Do not send `models: true` until the agent worker is implemented; extra flags are ignored today.

---

## Agent status (optional)

Per-model fields the controller may later read from agent status:

| Field | Meaning |
|-------|---------|
| `state` | `Pending`, `Pulling`, `Ready`, `Failed` |
| `digest` | OCI manifest digest after pull |
| `resolvedRevision` | HF commit (or resolved OCI ref) |
| `revisionFloating` | `true` when the requested revision is a floating tag/branch |

These are **optional** on status; omitting them must not break older controllers.

---

## Controller work remaining

- [ ] Registry REST: persist and return `type`, `ca`, `insecure`
- [ ] Model CRUD endpoints (create / read / update / delete)
- [ ] `getChanges` flag `models`
- [ ] Agent status optional model states (`state`, `digest`, `resolvedRevision`, `revisionFloating`)

Edgelet local API remains **`/v1/…`**. Pot controller REST remains **`/api/v3/…`** — no breaking path changes on either side for this handoff.
