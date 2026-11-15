# Duplicating notes and using samples

`POST /api/notes/{id}/duplicate`, MCP `duplicate_note(source_id, title?, parent_id?)`
and the UI's duplicate action share one server transaction. Omit parent_id to keep
the source parent; JSON null moves the copy to root. Title defaults to “(копия)”.
Content, colour, built-in/uploaded icon, privacy and all own typed values, formulas
and roll provenance are copied. Uploaded icons get independent files. Nothing is
rerolled or automatically checked; secrets, checks/bindings, popularity/history,
pin/category/readme/sample flags and source ordering are not copied. Local formula
names stay local; foreign entity references are not rewritten. Inheritance follows
the destination parent. The source is untouched. MCP refuses private sources and
private destination parents, including implicit original parents.

Embedded `attachment=N` references deliberately reuse the existing workspace-local
attachments. No byte copy/upload occurs, so storage is not charged twice. The source
files must remain available: deleting/purging an original attachment can break links
in any note, including a duplicate. Duplication does not claim independent file ownership.
An icon read/write failure or DB/value-copy failure rolls the whole creation back;
no success event or successful copy id is returned for a partial result.

`is_sample` marks an ordinary note as a reusable blueprint, unrelated to recurring
quest templates. Set/clear it in the note's Flags menu or MCP `set_note_flag` with
`flag: "sample"`; creation also accepts is_sample. Find samples with `flag:sample`
in UI search, or `list_notes(flags="sample")` / entity search. Lists expose title,
parent and brief description length; get_note reads its purpose and content.

The sample's action becomes “Создать по образцу”, offering a name and destination
parent. MCP uses duplicate_note with explicit overrides. A resulting card is an
independent note without the sample flag; later sample changes do not replace it.
No mandatory separate category is imposed. Typed parameter overrides can be sent
through set_values after creation; they are a separate atomic package, not a promise
of a cross-operation transaction. A multi-owner batch or a combined duplicate+override
transaction is deliberately outside this release.

## Sample protection

An existing sample is read-only: its title, Markdown, parent and other editable
metadata, own named values/formulas and secrets cannot be changed or deleted.
The note's sample flag can be changed alone; unmark it before editing. A request
that unmarks and edits at once is rejected. Creation and duplication still work,
and a duplicate is independent and editable. Inherited environment remains
inherited; this flag does not freeze the workspace or ancestor notes.

HTTP mutations return 403 with the note title. Map reference changes are text
edits: outgoing references from a sample are rejected, while other notes can
link to it. Draft admission checks the source, and storage triggers also protect
old drafts, scripts and concurrent flag changes. The UI displays a sample seal
in the tree and note heading, refuses text editing with a toast, and shows the
environment read-only. This protection is not a permissions boundary against
an administrator who can remove the flag.
