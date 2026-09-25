package com.svpc.ai;

import android.app.Activity;
import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.os.Build;
import android.os.Bundle;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.CookieManager;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

/**
 * SVPC AI for Android.
 *
 * The app is a thin client: the agent, its tools and the session store all run on
 * the machine that started the bridge, and this window is the same interface the
 * desktop application shows. That is deliberate — a phone cannot run the build,
 * shell or container tools the agent depends on, so the tools stay where the
 * code is.
 */
public class MainActivity extends Activity {

    private static final String PREFS = "svpc";
    private static final String KEY_HOST = "host";
    private static final String KEY_TOKEN = "token";

    private WebView web;
    private View setup;
    private EditText hostField;
    private EditText tokenField;
    private String lastUrl = "";

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        SharedPreferences prefs = getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        String host = prefs.getString(KEY_HOST, "");
        String token = prefs.getString(KEY_TOKEN, "");

        if (host.isEmpty() || token.isEmpty()) {
            showSetup(host, token);
        } else {
            openBridge(host, token);
        }
    }

    // ── Setup ──────────────────────────────────────────────────────
    //
    // The bridge address and token are shown once on the machine, so they have
    // to be typed in here. They are then remembered, so this appears once.

    private void showSetup(String host, String token) {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        int pad = dp(20);
        root.setPadding(pad, pad, pad, pad);
        root.setBackgroundColor(Color.parseColor("#0A0C11"));

        TextView title = new TextView(this);
        title.setText("SVPC AI");
        title.setTextColor(Color.parseColor("#E7EAF2"));
        title.setTextSize(24);
        root.addView(title);

        TextView blurb = new TextView(this);
        blurb.setText("Connect to the bridge running on your computer.\n"
                + "Start it there with:  svpc --serve 0.0.0.0\n"
                + "It prints the port and token to enter below.");
        blurb.setTextColor(Color.parseColor("#7C8497"));
        blurb.setTextSize(14);
        blurb.setPadding(0, dp(12), 0, dp(20));
        root.addView(blurb);

        hostField = new EditText(this);
        hostField.setHint("192.168.1.20:8080");
        hostField.setText(host);
        style(hostField);
        root.addView(hostField);

        tokenField = new EditText(this);
        tokenField.setHint("token");
        tokenField.setText(token);
        // The token is a shared secret; it should not appear on screen.
        tokenField.setInputType(android.text.InputType.TYPE_CLASS_TEXT
                | android.text.InputType.TYPE_TEXT_VARIATION_PASSWORD);
        style(tokenField);
        root.addView(tokenField);

        Button connect = new Button(this);
        connect.setText("Connect");
        connect.setOnClickListener(v -> {
            String h = hostField.getText().toString().trim();
            String t = tokenField.getText().toString().trim();
            if (h.isEmpty() || t.isEmpty()) {
                Toast.makeText(this, "Both the address and the token are needed.",
                        Toast.LENGTH_SHORT).show();
                return;
            }
            getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                    .putString(KEY_HOST, h)
                    .putString(KEY_TOKEN, t)
                    .apply();
            openBridge(h, t);
        });
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        lp.topMargin = dp(20);
        root.addView(connect, lp);

        setContentView(root);
        setup = root;
    }

    private void style(EditText field) {
        field.setTextColor(Color.parseColor("#E7EAF2"));
        field.setHintTextColor(Color.parseColor("#7C8497"));
        field.setBackgroundColor(Color.parseColor("#181D26"));
        field.setPadding(dp(12), dp(10), dp(12), dp(10));
        field.setSingleLine(true);
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        lp.bottomMargin = dp(10);
        field.setLayoutParams(lp);
    }

    // ── The bridge ─────────────────────────────────────────────────

    private void openBridge(String host, String token) {
        web = new WebView(this);
        web.setBackgroundColor(Color.parseColor("#0A0C11"));
        setContentView(web);

        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        // The agent streams its transcript as server-sent events, which needs a
        // real DOM rather than a reduced one.
        s.setDomStorageEnabled(true);
        s.setLoadWithOverviewMode(false);
        s.setUseWideViewPort(false);
        s.setBuiltInZoomControls(false);
        s.setSupportZoom(false);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            // Cleartext, because the bridge is reached over http on a local
            // network. The token is the credential, not TLS.
            s.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        }

        // The interface is a single page; navigating away would strand the user
        // in a view with no way back to the composer.
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                return !url.startsWith(baseUrl());
            }
        });

        CookieManager.getInstance().setAcceptCookie(true);

        lastUrl = "http://" + host + "/?token=" + token;
        web.loadUrl(lastUrl);
    }

    private String baseUrl() {
        SharedPreferences p = getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        String host = p.getString(KEY_HOST, "");
        return host.isEmpty() ? "http://" : "http://" + host + "/";
    }

    // ── Lifecycle ──────────────────────────────────────────────────

    private int dp(int value) {
        return (int) (value * getResources().getDisplayMetrics().density);
    }

    /**
     * Back goes into the page's own history before it leaves the app, so a
     * dismissed dialog or a dismissed permission card does not exit.
     */
    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        if (keyCode == KeyEvent.KEYCODE_BACK && web != null && web.canGoBack()) {
            web.goBack();
            return true;
        }
        if (keyCode == KeyEvent.KEYCODE_BACK) {
            // With no history left, offer the settings rather than closing: the
            // address is the one thing the user cannot guess.
            confirmReconfigure();
            return true;
        }
        return super.onKeyDown(keyCode, event);
    }

    private void confirmReconfigure() {
        android.app.AlertDialog.Builder b = new android.app.AlertDialog.Builder(this);
        b.setTitle("Connection");
        b.setMessage("Change the address or token?");
        b.setPositiveButton("Change", (d, w) -> {
            SharedPreferences p = getSharedPreferences(PREFS, Context.MODE_PRIVATE);
            showSetup(p.getString(KEY_HOST, ""), p.getString(KEY_TOKEN, ""));
        });
        b.setNegativeButton("Close the app", (d, w) -> finish());
        b.setNeutralButton("Stay", null);
        b.show();
    }

    @Override
    protected void onDestroy() {
        if (web != null) {
            web.destroy();
        }
        super.onDestroy();
    }
}
