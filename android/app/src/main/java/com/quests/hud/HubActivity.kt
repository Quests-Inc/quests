package com.quests.hud

import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity
import androidx.viewpager2.widget.ViewPager2
import com.quests.hud.data.PrefsStore
import com.quests.hud.databinding.ActivityHubBinding

/**
 * Swipeable hub: settings page + the SPA (quest=192). Replaces the old
 * separate MainActivity/QuestsWebActivity.
 */
class HubActivity : AppCompatActivity() {

    private lateinit var binding: ActivityHubBinding

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityHubBinding.inflate(layoutInflater)
        setContentView(binding.root)

        binding.hubPager.orientation = ViewPager2.ORIENTATION_HORIZONTAL
        binding.hubPager.offscreenPageLimit = 1
        binding.hubPager.adapter = HubPagerAdapter(this)
        // The SPA owns its gestures and requests settings through its JS bridge.
        // Settings uses SwipeCaptureLayout for the return gesture.
        binding.hubPager.isUserInputEnabled = false

        val startOnJournal = PrefsStore(this).isConfigured()
        if (startOnJournal) HubNav.pendingTab = HubNav.TAB_JOURNAL
        binding.hubPager.setCurrentItem(
            if (startOnJournal) HubPagerAdapter.PAGE_SPA else HubPagerAdapter.PAGE_SETTINGS,
            false,
        )
    }

    /** Settings → SPA, landing on "journal" (the common/forward case). */
    fun goToJournal() {
        HubNav.pendingTab = HubNav.TAB_JOURNAL
        binding.hubPager.setCurrentItem(HubPagerAdapter.PAGE_SPA, true)
    }

    /** Journal → settings (swiping right off the first SPA tab). */
    fun goToSettings() {
        binding.hubPager.setCurrentItem(HubPagerAdapter.PAGE_SETTINGS, true)
    }
}
