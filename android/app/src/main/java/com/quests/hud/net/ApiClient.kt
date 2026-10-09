package com.quests.hud.net

import android.util.Log
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.TimeUnit

class ApiError(message: String, val status: Int? = null) : IOException(message)

/**
 * Thin client for the same REST API the desktop overlay talks to (note=3):
 * GET /api/auth/state, GET /api/health, GET /api/quests, GET /api/events,
 * PATCH /api/quests/{id}. Bearer token = QUESTS_API_TOKEN.
 */
class ApiClient(private val base: String, private val token: String?) {

    suspend fun authState(): JSONObject = getJson("/api/auth/state", auth = false)

    suspend fun health(): JSONObject = getJson("/api/health", auth = true)

    suspend fun activeQuests(): List<JSONObject> {
        val body = request("GET", "/api/quests?status=active&status=frozen", auth = true)
        val arr = JSONArray(body)
        // overlay_hidden = the crossed-out pin: never shown in the HUD, the ongoing list included.
        return (0 until arr.length()).map { arr.getJSONObject(it) }
            .filterNot { it.optBoolean("overlay_hidden", false) }
    }

    /**
     * Durable event log + live buffer since [since] revision
     * (`GET /api/events?since=`). Used instead of raw WebSocket so we stay
     * on HttpURLConnection (no OkHttp) — quest=269.
     */
    suspend fun eventsSince(since: Int): Pair<Int, List<JSONObject>> {
        val body = request("GET", "/api/events?since=$since", auth = true)
        val json = JSONObject(body)
        val revision = json.optInt("revision", since)
        val arr = json.optJSONArray("events") ?: JSONArray()
        val events = (0 until arr.length()).map { arr.getJSONObject(it) }
        return revision to events
    }

    suspend fun patchQuest(questId: Int, body: JSONObject, quiet: Boolean = true) {
        val q = if (quiet) "?quiet=1" else ""
        request("PATCH", "/api/quests/$questId$q", auth = true, jsonBody = body)
    }

    private suspend fun getJson(path: String, auth: Boolean): JSONObject =
        JSONObject(request("GET", path, auth))

    private suspend fun request(
        method: String,
        path: String,
        auth: Boolean,
        jsonBody: JSONObject? = null,
    ): String = withContext(Dispatchers.IO) {
        val url = URL("$base$path")
        Log.i(TAG, "$method $url auth=$auth")
        val started = System.nanoTime()
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = method
        // Mobile → public edge gateway can take several seconds per hop;
        // 10s was timing out on SM-A366B against sslip.io (auth/state ~8s).
        conn.connectTimeout = TimeUnit.SECONDS.toMillis(30).toInt()
        conn.readTimeout = TimeUnit.SECONDS.toMillis(30).toInt()
        conn.setRequestProperty("Accept", "application/json")
        conn.setRequestProperty("Connection", "close")
        if (auth && !token.isNullOrBlank()) {
            conn.setRequestProperty("Authorization", "Bearer $token")
        }
        if (jsonBody != null) {
            conn.doOutput = true
            conn.setRequestProperty("Content-Type", "application/json; charset=utf-8")
            conn.outputStream.bufferedWriter(Charsets.UTF_8).use { it.write(jsonBody.toString()) }
        }
        try {
            val status = conn.responseCode
            val stream = if (status in 200..299) conn.inputStream else conn.errorStream
            val text = stream?.bufferedReader()?.use { it.readText() } ?: ""
            val ms = (System.nanoTime() - started) / 1_000_000
            Log.i(TAG, "$method $path → $status in ${ms}ms (${text.length} bytes)")
            if (status !in 200..299) {
                throw ApiError("API $status: $text", status)
            }
            text
        } catch (e: Exception) {
            val ms = (System.nanoTime() - started) / 1_000_000
            Log.e(TAG, "$method $path failed after ${ms}ms: ${e.javaClass.simpleName}: ${e.message}")
            throw e
        } finally {
            conn.disconnect()
        }
    }

    private companion object {
        const val TAG = "QuestsApi"
    }
}
