package com.quests.hud

/**
 * Tiny handoff between the settings page and the SPA-hub page: which SPA
 * tab to land on when the pager brings the SPA hub into view. Set right
 * before `setCurrentItem(1, ...)`, read once in WebHubFragment.onResume and
 * cleared. quest=192.
 */
object HubNav {
    const val TAB_JOURNAL = "journal"
    // Linear, no wrap: settings through calendar.
    val TAB_ORDER = listOf("journal", "toc", "notes", "attachments", "calendar")

    var pendingTab: String? = null

    /** Set by the settings page (after saving, or its reload button): the
     * SPA hub reloads the page on its next onResume even if the base URL is
     * unchanged. Read once and cleared. */
    var reloadRequested = false
}
