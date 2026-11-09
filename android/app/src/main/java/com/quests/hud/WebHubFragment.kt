package com.quests.hud

import android.annotation.SuppressLint
import android.os.Bundle
import android.webkit.JavascriptInterface
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import androidx.fragment.app.Fragment
import com.quests.hud.data.PrefsStore
import com.quests.hud.databinding.FragmentWebHubBinding
import org.json.JSONObject

/** SPA gestures belong to the page; only its start boundary opens native settings. */
class WebHubFragment : Fragment() {

    private var _binding: FragmentWebHubBinding? = null
    private val binding get() = _binding!!
    /** Base URL the WebView last loaded; a different one in prefs means the
     * server address was changed in settings and the page must be reloaded —
     * otherwise it keeps talking to the old host (e.g. a Tailscale IP that's
     * unreachable without Tailscale → ERR_TIMED_OUT). */
    private var loadedBase: String? = null
    override fun onCreateView(
        inflater: LayoutInflater,
        container: ViewGroup?,
        savedInstanceState: Bundle?,
    ): View {
        _binding = FragmentWebHubBinding.inflate(inflater, container, false)
        return binding.root
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)

        binding.spaWebView.settings.javaScriptEnabled = true
        binding.spaWebView.settings.domStorageEnabled = true
        // This WebView's HTTP cache survives across app runs and can pin an
        // old bundle from a previous debug install — e.g. one predating the
        // questsNav hook this fragment depends on. Always revalidate.
        binding.spaWebView.settings.cacheMode = android.webkit.WebSettings.LOAD_NO_CACHE
        binding.spaWebView.clearCache(true)
        binding.spaWebView.webViewClient = android.webkit.WebViewClient()

        binding.spaWebView.addJavascriptInterface(SettingsBridge(), "QuestsAndroid")
    }

    override fun onResume() {
        super.onResume()
        val landingTab = HubNav.pendingTab ?: HubNav.TAB_JOURNAL
        HubNav.pendingTab = null

        val base = PrefsStore(requireContext()).apiBase?.trim()?.trimEnd('/')
        if (base.isNullOrBlank()) return

        if (base != loadedBase || HubNav.reloadRequested) {
            HubNav.reloadRequested = false
            loadedBase = base
            binding.spaWebView.loadUrl(if (landingTab == "journal") base else "$base/?tab=$landingTab")
        } else {
            setTab(landingTab)
        }
    }

    private inner class SettingsBridge {
        @JavascriptInterface
        fun openSettings() {
            // JS bridge calls arrive on WebView's background thread.
            activity?.runOnUiThread {
                if (_binding != null) (activity as? HubActivity)?.goToSettings()
            }
        }
    }

    private fun setTab(tab: String) {
        binding.spaWebView.evaluateJavascript(
            "window.questsNav && window.questsNav.setTab(${JSONObject.quote(tab)});",
            null,
        )
    }

    override fun onDestroyView() {
        // A recreated view gets a fresh, empty WebView — load it again.
        binding.spaWebView.removeJavascriptInterface("QuestsAndroid")
        loadedBase = null
        _binding = null
        super.onDestroyView()
    }
}
