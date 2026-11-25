//go:build linux && cgo && gtk3

package linux

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

extern gboolean flMayRecord(char *page, gboolean audio, gboolean video, gboolean display);

// fl_load_changed keeps the address a web view committed to: the page it shows,
// which a load under way does not change until it commits.
static void fl_load_changed(WebKitWebView *view, WebKitLoadEvent event, gpointer data) {
	if (event == WEBKIT_LOAD_COMMITTED)
		g_object_set_data_full(G_OBJECT(view), "freeloop-page", g_strdup(webkit_web_view_get_uri(view)), g_free);
}

// fl_permission_request answers every request for the microphone, camera or
// screen and ends the signal there, before Wails' own handler, which would allow
// whatever page the window shows. Other requests go on to Wails and to WebKit,
// which deny them.
static gboolean fl_permission_request(WebKitWebView *view, WebKitPermissionRequest *request, gpointer data) {
	if (!WEBKIT_IS_USER_MEDIA_PERMISSION_REQUEST(request))
		return FALSE;
	WebKitUserMediaPermissionRequest *media = WEBKIT_USER_MEDIA_PERMISSION_REQUEST(request);
	if (flMayRecord(g_object_get_data(G_OBJECT(view), "freeloop-page"),
			webkit_user_media_permission_is_for_audio_device(media),
			webkit_user_media_permission_is_for_video_device(media),
			webkit_user_media_permission_is_for_display_device(media)))
		webkit_permission_request_allow(request);
	else
		webkit_permission_request_deny(request);
	return TRUE;
}

// fl_parent_set sees every widget put into another: a web view, the first time,
// gets the media stream and the two handlers above. Wails packs its web view into
// the window before it connects its own handlers and before it loads the page.
static gboolean fl_parent_set(GSignalInvocationHint *hint, guint n, const GValue *values, gpointer data) {
	GObject *object = g_value_get_object(&values[0]);
	if (!WEBKIT_IS_WEB_VIEW(object) || g_object_get_data(object, "freeloop-voice"))
		return TRUE;
	g_object_set_data(object, "freeloop-voice", GINT_TO_POINTER(1));
	webkit_settings_set_enable_media_stream(webkit_web_view_get_settings(WEBKIT_WEB_VIEW(object)), TRUE);
	g_signal_connect(object, "load-changed", G_CALLBACK(fl_load_changed), NULL);
	g_signal_connect(object, "permission-request", G_CALLBACK(fl_permission_request), NULL);
	return TRUE;
}

static void fl_watch_web_views(void) {
	// The class holds the signal; it stays referenced for the app's life.
	g_type_class_ref(GTK_TYPE_WIDGET);
	g_signal_add_emission_hook(g_signal_lookup("parent-set", GTK_TYPE_WIDGET), 0, fl_parent_set, NULL, NULL);
}
*/
import "C"

import "sync"

// The app's Linux build has Wails' gtk3 tag, for GTK 3 and WebKitGTK 4.1, as
// here; webview_nogtk3.go is the rest.

var watching sync.Once

// PrepareWebViews turns on the microphone for voice input (GUI-050, OQ-048) in
// every web view the app makes from then on, for the app's own page only
// (mayRecord). It runs before the first window opens, on the thread GTK runs on;
// a WebKitGTK web view reads enable-media-stream when its page loads.
func PrepareWebViews() {
	watching.Do(func() { C.fl_watch_web_views() })
}

//export flMayRecord
func flMayRecord(page *C.char, audio, video, display C.gboolean) C.gboolean {
	if mayRecord(C.GoString(page), audio != 0, video != 0, display != 0) {
		return 1
	}
	return 0
}
