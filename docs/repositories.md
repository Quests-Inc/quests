# Workspace repositories

A repository belongs to a workspace. Quests, questlines and notes can link to it, following attachment ownership. Steps cannot own repositories. Committing or pushing never completes a quest step.

**Data → Repositories** is the workspace catalog. Quest/questline/note cards have a compact collapsible list of repository links. Tree browsing is restricted to the dedicated catalog tab. Create a repository in place, or attach an existing one. The dedicated tab shows the clone URL, branch and commit selectors (the last 30 commits), and a directory tree at the selected revision. Files are listed but cannot be opened or edited in this MVP. A later file preview can reuse the existing note presentation primitives. “Unlink” removes the association, not the source repository.

## Local Git

Repositories are bare directories under `data/repositories/WORKSPACE/ID.git`. Clone/push uses Git smart HTTP at `/api/repositories/ID/git`, implemented by the fixed `git http-backend` executable. The API image includes Git. Use the actual Quests account login as Basic-auth username and its API token **pinned to that workspace** as the password. The server rejects a username that differs from the token owner, or configure an Authorization: Bearer header in the client. Tokens are never included in the clone URL. Reads require membership; creation, linking, secret changes and push require a workspace owner. The bootstrap auth-off loopback stand uses its existing trusted mode.

The default aggregate workspace quota is **1 GiB of bare-directory bytes**, independent of attachment revisions/quota. New repositories count toward it. A push runs against a private staged copy under a workspace lock. Only a successful response within quota replaces the current directory; rejected oversized pushes leave refs and bytes unchanged. Staging needs temporary disk headroom, potentially another repository-sized copy. This implementation assumes one API process owns the data directory.

Server-side hooks and automatic maintenance are disabled in the Git environment. No working tree is checked out. Only fixed plumbing commands and smart upload/receive-pack endpoints run; repository contents, filenames and commit messages are data, not shell commands. Dumb HTTP/file access, CI, LFS, pull requests, push proxying and arbitrary server commands are outside this feature.

## GitHub / GitLab

External repositories store an HTTPS repository URL (`github.com/owner/repo` or `gitlab.com/group/project`, including nested GitLab groups) and an optional encrypted read token. Push goes directly to the host; Quests reads branches, commits and contents via provider APIs. The token has a dedicated table and is never included in normal API reads or runner environments. An empty replacement token removes it.

Provider API access uses the server's existing egress policy and limits. The workspace must have the existing administrator egress grant; the operator must also allow `api.github.com` and/or `gitlab.com`; disabled egress gives an explicit error rather than bypassing policy. Credentials may follow redirects only within the original origin. Self-hosted providers are not supported in this first implementation. Branch listing and GitLab directory listing currently use a 100-item provider page; local Git tree output and tree output are bounded by the 4 MiB Git-output cap. Large/binary files can be read from a local clone.

References: [Git HTTP backend](https://git-scm.com/docs/git-http-backend), [GitHub contents API](https://docs.github.com/en/rest/repos/contents), [GitLab repositories API](https://docs.gitlab.com/api/repositories/).

## Workspace archives

Metadata and associations travel in the workspace archive with remapped IDs. Local Git bytes use `repositories/OLD_ID.bundle` (reachable refs/objects), independently of the attachment-file export option. Empty repositories use metadata only. Import initializes a clean bare repository and fetches the bundle; archived config/hooks are never installed. Repository tokens travel only when secrets are explicitly included, using the existing archive plaintext/re-encryption contract. Import failure removes the new workspace and its repository directory. Scheduled permanent workspace purge also removes its repository directory after the existing deletion grace period.

Migration **0046** removes earlier draft step links/path fields and permits notes; no source bytes are removed. Migration **0045** adds repository, repository_link and repository_token with workspace RLS. The application role never accesses another workspace's source directory until the repository row passed its workspace lookup.

API tokens belong to an account (`apitoken.user_id`) and are scoped to a workspace; they are not shared workspace identities. Basic username matching catches mismatched credentials, while the token remains the actual secret. Accepted Git HTTP requests use the verified username as REMOTE_USER and successful pushes log the account/workspace/repository. Client-supplied commit author/committer metadata is separate from the authenticated pusher and is not proof of identity.

## Stable Git addresses

Migration **0047** adds `workspace.slug` (unique on the server, including deleted
workspaces until purge) and `repository.slug` (unique within a workspace). Both
accept 1–63 lowercase ASCII letters/digits separated by single hyphens. Existing
and newly created objects default to `workspace-ID` / `repo-ID`, so duplicate or
non-Latin display titles need no automatic transliteration. Slugs are independent
of display titles.

Owners edit the workspace slug in Settings → Workspace, and the repository slug
in its creation/settings dialog. Changing a slug changes the named clone URL;
the form warns to update local `origin`. Old slug names are not redirects. The
original ID-based `/api/repositories/ID/git` URL remains supported.

The canonical local clone URL is `/git/WORKSPACE_SLUG/REPOSITORY_SLUG.git`.
Authentication is the same verified account login + workspace-scoped API token.
The named URL selects the workspace, but never expands the token's workspace
scope or membership/owner permissions. Git paths are authenticated API routes,
not public SPA assets. Vite proxies `/git` to the API as well as `/api`.

The repository view provides a collapsible shortcut recipe, for example:

```sh
git config --global url."https://quests.example/git/my-space/".insteadOf "q:"
git clone q:my-project.git
```

This is optional client configuration; Quests never edits the user's global Git
settings. Use distinct prefixes for different spaces if configuring several.

## Navigation and provider request budget

Right-click a catalog row or its title to copy `repo=ID`, edit the repository,
copy its clone/source URL, or copy the full card URL. Repository mentions render
as named links in Markdown and short text, navigate to `?repo=ID`, and have their
IDs rewritten during workspace archive import.

The browser coalesces and caches up to 128 visited directory/branch/commit views
in memory until reload/logout; successful edits invalidate the affected views.
The inexpensive workspace catalog is shared for 30 seconds and supplies link
labels. No provider credentials or private tree payloads are persisted in browser
storage. A consumer leaving the page does not cancel another consumer's request.

The server also coalesces provider reads and caches successful JSON responses for
10 minutes (up to 512 responses / 16 MiB per process). ETags are retained for up
to 30 minutes for conditional validation. Permission/egress checks and credential
reads precede every lookup; cache keys include workspace, repository URL, and a
credential fingerprint. Changing/removing a token cannot reuse another token's
cache. The refresh button revalidates branch lists and recent commits; unchanged
SHA trees/path history stay cached. Provider errors are not successful cache hits.

GitHub's recursive Git-tree endpoint loads all directories at the selected SHA
with one provider request. Truncated responses or the existing egress body-size
limit trigger directory-by-directory fallback; the oversized-tree outcome is
remembered, avoiding repeated expensive probes. GitLab uses cached directory
pages. Neither implementation downloads file contents or the entire Git history.
The selector retains the last 30 commits. Per-path last-commit metadata is fetched
only for the opened directory, with two concurrent calls and a ten-second total
budget. Missing history stays marked partial instead of using a fabricated date.

GitHub normally permits 60 unauthenticated reads/hour per IP or 5000 with a PAT
(shared across the account's clients). Secondary limits also apply. A rate-limit
response stops further uncached calls for that credential/provider until
`Retry-After` / `X-RateLimit-Reset` permits retry (at least one minute without
headers). There is no automatic retry loop. Authenticated conditional `304`
responses do not consume GitHub's primary quota, but still use Quests' independent
workspace egress budget; caching never expands that administrator policy.

References: [GitHub rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api),
[API best practices](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api),
[recursive trees and truncation](https://docs.github.com/en/rest/git/trees).

External tokens are edited in the main repository settings dialog. A blank field
preserves the current encrypted token, a new value replaces it, and a separate
checkbox explicitly removes it. The stored value is never returned to the form.
The dialog includes provider-specific token creation instructions: GitHub
fine-grained PAT with selected repository + Contents read-only; GitLab read_api
for the branches/commits/tree API endpoints. Private repositories work when the
token has access (including required organization approval). Repository tree and
history are shared with the workspace's members; this token is not a per-member
provider authorization.
