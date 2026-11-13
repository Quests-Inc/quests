# Workspace repositories

A repository belongs to a workspace. Multiple quests, questlines and steps can link to it; a step link can name a file or directory. Committing or pushing never completes a quest step.

**Data → Repositories** is the workspace catalog. Quest/questline cards have a collapsible Repositories block; steps have the same block with a path field. Create a repository in place, or attach an existing one. Opening the card shows clone URL, branch, the last 30 commits, a navigable tree and plain-text file content. Contents are never rendered as executable HTML. “Unlink” removes the association, not the source repository.

## Local Git

Repositories are bare directories under `data/repositories/WORKSPACE/ID.git`. Clone/push uses Git smart HTTP at `/api/repositories/ID/git`, implemented by the fixed `git http-backend` executable. The API image includes Git. Use any Basic-auth username and a Quests API token **pinned to that workspace** as the password, or configure an Authorization: Bearer header in the client. Tokens are never included in the clone URL. Reads require membership; creation, linking, secret changes and push require a workspace owner. The bootstrap auth-off loopback stand uses its existing trusted mode.

The default aggregate workspace quota is **256 MiB of bare-directory bytes**, independent of attachment revisions/quota. New repositories count toward it. A push runs against a private staged copy under a workspace lock. Only a successful response within quota replaces the current directory; rejected oversized pushes leave refs and bytes unchanged. Staging needs temporary disk headroom, potentially another repository-sized copy. This implementation assumes one API process owns the data directory.

Server-side hooks and automatic maintenance are disabled in the Git environment. No working tree is checked out. Only fixed plumbing commands and smart upload/receive-pack endpoints run; repository contents, filenames and commit messages are data, not shell commands. Dumb HTTP/file access, CI, LFS, pull requests, push proxying and arbitrary server commands are outside this feature.

## GitHub / GitLab

External repositories store an HTTPS repository URL (`github.com/owner/repo` or `gitlab.com/group/project`, including nested GitLab groups) and an optional encrypted read token. Push goes directly to the host; Quests reads branches, commits and contents via provider APIs. The token has a dedicated table and is never included in normal API reads or runner environments. An empty replacement token removes it.

Provider API access uses the server's existing egress policy and limits. The workspace must have the existing administrator egress grant; the operator must also allow `api.github.com` and/or `gitlab.com`; disabled egress gives an explicit error rather than bypassing policy. Credentials may follow redirects only within the original origin. Self-hosted providers are not supported in this first implementation. Branch listing and GitLab directory listing currently use a 100-item provider page; local Git tree output and text-file display are bounded by the 4 MiB Git-output cap. Large/binary files can be read from a local clone.

References: [Git HTTP backend](https://git-scm.com/docs/git-http-backend), [GitHub contents API](https://docs.github.com/en/rest/repos/contents), [GitLab repositories API](https://docs.gitlab.com/api/repositories/).

## Workspace archives

Metadata, associations and step paths travel in the workspace archive with remapped IDs. Local Git bytes use `repositories/OLD_ID.bundle` (reachable refs/objects), independently of the attachment-file export option. Empty repositories use metadata only. Import initializes a clean bare repository and fetches the bundle; archived config/hooks are never installed. Repository tokens travel only when secrets are explicitly included, using the existing archive plaintext/re-encryption contract. Import failure removes the new workspace and its repository directory. Scheduled permanent workspace purge also removes its repository directory after the existing deletion grace period.

Migration **0045** adds repository, repository_link and repository_token with workspace RLS. The application role never accesses another workspace's source directory until the repository row passed its workspace lookup.
