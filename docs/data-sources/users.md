---
layout: data-source
page_title: "openwebui_users Data Source"
sidebar_current: docs-openwebui-data-source-users
description: |-
  Lists users in Open WebUI.
---

# openwebui_users (Data Source)

Fetches the list of users, optionally filtered by a search query. Paginates through
all matching results automatically.

## Example Usage

### Minimal

```hcl
data "openwebui_users" "all" {}
```

### Full

```hcl
data "openwebui_users" "engineers" {
  query = "engineering"
}
```

## Argument Reference

* `query` (Optional) – Search query used to filter users by username, email, or name. If omitted, all users are returned.

## Attribute Reference

* `users` – List of users matching the query. Each item includes `id`, `name`, `email`, `username`, `role`, `profile_image_url`, `bio`, `last_active_at`, `updated_at`, and `created_at`.
