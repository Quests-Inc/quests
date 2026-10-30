# Attachment previews

Cards and the selected-file panel show inert excerpts alongside the existing raster image previews. Only text/code is fetched automatically near the viewport; raster images use native lazy loading. PDF, office documents, archives and other formats show a keyboard-accessible “Показать превью” button and make no preview request until clicked, both in cards and the selected-file panel. Clicking it does not select the card. Results stay in the mounted component (no shared in-memory cache); empty/failed results show “Превью недоступно”. Changing files or revisions resets manual consent and aborts the previous request. Preview failures keep the file icon and do not emit toasts.

`GET /api/attachments/{id}/preview?revision=N` returns `{kind,text}`. It uses the same workspace visibility as attachment downloads and serves JSON, never uploaded HTML or SVG as executable content. Clean scan status is required. Browser caching is private and revalidated against an ETag including attachment ID, revision, size and upload time. Failed/unsupported previews have no ETag.

Supported:

- Text, source code, Markdown, CSV/TSV, JSON, XML, HTML and SVG: first 64 KiB, UTF-8 or UTF-16 with BOM, rendered literally.
- PDF: first-page text via `pdftotext`, without OCR. Scanned/image-only pages fall back to the icon.
- DOCX/PPTX/XLSX and ODT/ODS/ODP: text from selected XML entries, without layout or formula evaluation. Spreadsheet excerpts are approximate raw cell/shared-string content.
- ZIP: up to 200 entry names, without unpacking files to disk.

Output is limited to 2000 characters. PDF/ZIP/office input is capped at 8 MiB; office XML decompression at 1 MiB. The server admits at most two preview requests concurrently with a four-second deadline per request. Unsupported, oversized, unavailable or malformed files return empty text.

The API Docker image installs `poppler-utils`. For a native development API, install Poppler (`poppler` on Arch, `poppler-utils` on Debian/Ubuntu); if unavailable, PDF cards retain their file icon. There is no OCR, headless office renderer, or conversion of media files.
