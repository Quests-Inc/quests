"""Render one HTML file to PDF with every external fetch refused.

The HTML comes from any logged-in user. WeasyPrint would otherwise follow
file:// and http:// references in it (<img>, <link rel=attachment>, CSS url(),
@import, <object>) and embed what it finds in the PDF handed back to the
caller: local files, the server's environment, internal services. Only data:
URIs are allowed, and the document gets no base URL.
"""
import sys

from weasyprint import HTML

try:  # WeasyPrint >= 68: a fetcher object with a protocol allow-list
    from weasyprint.urls import URLFetcher

    FETCHER = URLFetcher(allowed_protocols=["data"], allow_redirects=False)
except ImportError:  # older releases take a plain function

    def FETCHER(url, *args, **kwargs):
        if url.startswith("data:"):
            from weasyprint import default_url_fetcher

            return default_url_fetcher(url, *args, **kwargs)
        raise ValueError("external resources are not allowed in exports")


def main(src, dst):
    with open(src, encoding="utf-8") as fh:
        text = fh.read()
    HTML(string=text, base_url=None, url_fetcher=FETCHER).write_pdf(dst)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
