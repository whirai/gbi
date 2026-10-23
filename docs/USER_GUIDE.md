# gbi User Guide

This guide is for business users and administrators covering day-to-day use of gbi. For installation and operations, read the [Deployment Guide](DEPLOYMENT.md); for an overview see [../README.md](../README.md).

> English README: [../README.md](../README.md) · 中文 README: [../README.zh-CN.md](../README.zh-CN.md) · 中文用户指南: [USER_GUIDE.zh-CN.md](USER_GUIDE.zh-CN.md)

---

## 1. Roles and sign-in

| Role | Capabilities |
|------|--------------|
| **admin** | Everything: users/departments/groups/roles, data subjects, BI pages, navigation menus, LLM settings, branding |
| **developer** | Data subjects, BI pages, navigation menus, AI Studio; can access the portal |
| **user** | Signs in and uses the BI portal; sees published pages granted via ACL |

Sign in at the deployment URL (default `http://localhost:8080`). The seeded administrator is **admin / admin123** — reset the password under User Management after the first login.

> Forgot your password: an administrator resets it via User Management → Reset password; make sure at least one usable administrator account remains.

## 2. Five-minute quick start

1. **Configure the LLM** (admin): Console → **LLM Settings** → edit the seeded provider, enter the API key, confirm the model name → save.
2. **Create a data subject**: Console → **Data Subjects** → New; provide a name and code (code must start with a lowercase letter, contain only lowercase letters/digits/underscores, ≤32 chars).
3. **Import data**: select the subject → **Import Excel** → choose a file and mode (Create a new table / Append to an existing table).
4. **Create a BI page**: **BI Pages** → New page → click **Open Studio** on its row.
5. **Let the AI build charts**: in Studio, select the data subjects for this analysis → describe what you need → send. The model inspects schemas, tries SQL, and generates charts.
6. **Publish and expose in the portal**: back on the page list → **Publish** → **Permissions** to grant users/groups → **Navigation Menus** to place the page in a menu.
7. **Open the portal**: visit `/portal` (or "Open BI Portal"), browse the menu.

## 3. Admin console

### 3.1 Users, departments, groups, roles

- **Users**: create (username, password, name, department, role), edit, reset password, delete;
- **Departments**: maintain the department tree;
- **Groups**: manage group members; page ACL can grant access to groups;
- **Roles**: built-in admin/developer/user; names can be changed (capabilities are defined by system policy).

### 3.2 Data subjects

A data subject is a container of related tables; each subject owns an isolated SQLite database.

- Create a subject: name + code (the code is used as the database prefix in SQL and should be chosen carefully);
- A subject can hold multiple tables, switched via tabs;
- **New table**: choose the table kind and define columns.
  - **DWD** (detail data): physical name is prefixed, e.g. base name `orders` → `dwd_orders`;
  - **DIM** (dimension data): `dim_<base name>`;
- **Import Excel** (2GB per upload):
  - `Create a new table`: reads the first worksheet; the first row is the header; column types are inferred (TEXT/INTEGER/REAL/DATE);
  - `Append to an existing table`: matches columns by name and appends rows; names must match the target table;
  - Tips: keep header names unique and non-empty; keep numeric columns numeric; empty cells become NULL;
- **Delete table**: removes all data in the table; a confirmation dialog is shown.

### 3.3 Table detail (data browsing)

Select a subject, then click a table tab:

- **Edit cells**: type directly in a cell; save on blur/Enter; numeric columns are validated;
- **Delete rows**: the "Delete" action on a row, with confirmation;
- **Column filters** (server-side): click the funnel icon in a header, tick one or more values, then confirm; "Clear filters" resets. Each column lists at most 100 distinct values;
- **Column settings**: choose which columns to show, with select all/none; remembered per browser;
- **Pagination**: page sizes 10/20/50/100;
- **Wide tables**: scroll horizontally inside the table while the "#" and "Actions" columns stay pinned; hover a truncated header to see the full name, type, and comment.

### 3.4 BI pages

- **New page**: name and description; new pages start as drafts;
- **Open Studio**: opens the AI Studio for that page (see section 4);
- **Publish / Unpublish**: only published pages granted via ACL appear in the portal;
- **Permissions (ACL)**: grant page visibility to **users** and **user groups**;
- **Versions**: review history and restore an older configuration (a version is created on each page update);
- **Delete**: removes the page and its versions.

### 3.5 Navigation menus

The portal navigation consists of menus and menu items:

1. Create a **menu** (e.g., "Sales Analytics");
2. Add **menu items** under it, each linking a BI page;
3. The portal shows menus as top-level tabs, with the menu's pages listed underneath.

Menus and pages the user is not authorized to see are hidden.

### 3.6 LLM settings (admin only)

Manage the model used by AI Studio:

- **Provider**: name, Base URL (**must be an Anthropic-compatible endpoint**, e.g. `https://api.deepseek.com/anthropic`), API key (only a mask is shown afterwards; leave the field blank when editing to keep the current key);
- **Model**: name, context window (default 384000), max output (default 16384);
  - constraint: **max output must be greater than 8192** (the thinking budget) and **less than the context window**;
- **Set as current**: there is a single global "current model"; switching takes effect on the next chat without restart;
- Initialization seeds a `deepseek` provider and a `deepseek-flash` model but **without an API key** — fill it in. If your endpoint does not serve that model name, change it to a real one;
- Delete protection: the current model cannot be deleted; a provider with models requires deleting its models first.

### 3.7 Branding (admin only)

- Change the **system name** and **browser title** (applies site-wide, including the login page);
- Upload a **logo** and **favicon**: only **PNG / ICO**, at most **512KB** each, with preview;
- Reset a single image or "Reset all to defaults" (names return to "生成式 BI", images fall back to the built-in favicon);
- Changes apply immediately site-wide (the favicon uses a versioned URL, so no cache clearing is needed).

## 4. AI Studio

Entry: **BI Pages** → **Open Studio** on a row, or visit `/studio?page=<page id>`.

Workflow:

1. Select one or more **data subjects** at the top (the AI can only access the subjects you select);
2. Describe your request (e.g., "Monthly sales by province, bar chart plus a trend line") and send;
3. The assistant: inspects schemas and sample rows → writes and **tries read-only SQL** → updates the page chart configuration → automatically **verifies rendering**; if the frontend reports chart render errors, they are fed back so the model can fix them;
4. The page preview updates live; when satisfied, go back to the page list and publish.

Notes and limits:

- Queries allow a single `SELECT`/`WITH` statement, capped at 10,000 rows; at most 200 rows are returned to the model (with a total-row count when truncated);
- Conversations are stored per page and survive reloads; "New conversation" clears only the chat history and **does not** affect the generated page configuration;
- If the model claims success but charts are blank, ask it to "check page render errors";
- If the model is unavailable, check the current model and API key in **LLM Settings**.

## 5. BI portal

- Visit `/portal` (after signing in);
- Top menus correspond to menus created under **Navigation Menus**; items link pages;
- Only **published** pages **granted to you or your groups** are listed;
- A specific page can be opened directly (e.g., `/portal?page=<page id>`, permission required);
- Charts in the portal match what you saw in Studio; admins/developers can jump to the console from the portal.

## 6. FAQ

| Problem | Resolution |
|---------|------------|
| AI does not respond or errors | Check **LLM Settings**: a current model exists, the API key is configured, the base URL is Anthropic-compatible; max output must be >8192 and < context window |
| Charts blank or broken | In Studio ask to "check page render errors"; the usual cause is chart `encode` fields not matching SQL result columns — let the AI fix them |
| Excel import fails | Check for empty/duplicate header names; in Append mode, column names must match the target table; ensure the file is within 2GB and the proxy upload limit |
| Column filter has no options | The column has no valid data (all NULL/empty) or more than 100 distinct values (only the first 100 are listed) |
| A page is missing | Is the page published, granted to you or your group under Permissions, and included in a menu? |
| 401/403 after signing in | Session expired or the account is disabled; sign in again; contact an administrator if it persists |
| Branding/model changes not visible | Branding and current-model switches are immediate; refresh stale tabs; the new model is used from the next chat |
| Accidentally deleted a table/row | Table deletion is irreversible; row/cell deletion cannot be undone. Back up regularly per the Deployment Guide |

More operational topics (ports, reverse proxy, upgrades, backups) are covered in the [Deployment Guide](DEPLOYMENT.md).
