#include "listener.h"
#include <unistd.h>
#include <stdlib.h>
#include <string.h>

struct _PhaluneAgentListener
{
  PolkitAgentListener parent_instance;
};

G_DEFINE_TYPE(PhaluneAgentListener, phalune_agent_listener, POLKIT_AGENT_TYPE_LISTENER)

typedef struct {
  uintptr_t id;
  GTask *task;
  GCancellable *cancellable;
  gulong cancel_handler_id;
  GList *identities;
  PolkitAgentSession *session;
  gulong req_handler;
  gulong err_handler;
  gulong info_handler;
  gulong comp_handler;
} AuthTask;

static GHashTable *active_tasks = NULL;
static uintptr_t next_task_id = 1;

static void on_task_cancelled(GCancellable *cancellable, gpointer user_data)
{
  AuthTask *at = (AuthTask *)user_data;
  if (at != NULL) {
    goTaskCancelled(at->id);
  }
}

static void on_session_request(PolkitAgentSession *session,
                               gchar              *request,
                               gboolean            echo_on,
                               gpointer            user_data)
{
  AuthTask *at = (AuthTask *)user_data;
  if (at != NULL) {
    goSessionRequest(at->id, request ? request : "", echo_on ? 1 : 0);
  }
}

static void on_session_show_error(PolkitAgentSession *session,
                                  gchar              *text,
                                  gpointer            user_data)
{
  AuthTask *at = (AuthTask *)user_data;
  if (at != NULL) {
    goSessionShowError(at->id, text ? text : "");
  }
}

static void on_session_show_info(PolkitAgentSession *session,
                                 gchar              *text,
                                 gpointer            user_data)
{
  AuthTask *at = (AuthTask *)user_data;
  if (at != NULL) {
    goSessionShowInfo(at->id, text ? text : "");
  }
}

static void on_session_completed(PolkitAgentSession *session,
                                 gboolean            gained_authorization,
                                 gpointer            user_data)
{
  AuthTask *at = (AuthTask *)user_data;
  if (at != NULL) {
    goSessionCompleted(at->id, gained_authorization ? 1 : 0);
  }
}

static void auth_task_clean_session(AuthTask *at)
{
  if (at->session != NULL) {
    if (at->req_handler > 0) g_signal_handler_disconnect(at->session, at->req_handler);
    if (at->err_handler > 0) g_signal_handler_disconnect(at->session, at->err_handler);
    if (at->info_handler > 0) g_signal_handler_disconnect(at->session, at->info_handler);
    if (at->comp_handler > 0) g_signal_handler_disconnect(at->session, at->comp_handler);
    at->req_handler = 0;
    at->err_handler = 0;
    at->info_handler = 0;
    at->comp_handler = 0;

    g_object_unref(at->session);
    at->session = NULL;
  }
}

static void auth_task_free(AuthTask *at)
{
  if (at == NULL) return;

  if (at->cancellable != NULL && at->cancel_handler_id > 0) {
    g_cancellable_disconnect(at->cancellable, at->cancel_handler_id);
    at->cancel_handler_id = 0;
  }

  auth_task_clean_session(at);

  if (at->identities != NULL) {
    g_list_free_full(at->identities, g_object_unref);
    at->identities = NULL;
  }

  if (at->cancellable != NULL) {
    g_object_unref(at->cancellable);
    at->cancellable = NULL;
  }

  if (at->task != NULL) {
    g_object_unref(at->task);
    at->task = NULL;
  }

  g_free(at);
}

static void phalune_agent_listener_initiate_authentication(
    PolkitAgentListener  *listener,
    const gchar          *action_id,
    const gchar          *message,
    const gchar          *icon_name,
    PolkitDetails        *details,
    const gchar          *cookie,
    GList                *identities,
    GCancellable         *cancellable,
    GAsyncReadyCallback   callback,
    gpointer              user_data)
{
  if (active_tasks == NULL) {
    active_tasks = g_hash_table_new_full(g_direct_hash, g_direct_equal, NULL, (GDestroyNotify)auth_task_free);
  }

  AuthTask *at = g_new0(AuthTask, 1);
  at->id = next_task_id++;
  at->task = g_task_new(G_OBJECT(listener), cancellable, callback, user_data);

  if (cancellable != NULL) {
    at->cancellable = g_object_ref(cancellable);
    at->cancel_handler_id = g_cancellable_connect(cancellable, G_CALLBACK(on_task_cancelled), at, NULL);
  }

  // Copy identities
  at->identities = g_list_copy_deep(identities, (GCopyFunc)g_object_ref, NULL);

  g_hash_table_insert(active_tasks, (gpointer)at->id, at);

  int num_identities = g_list_length(identities);
  char **ident_strings = g_new0(char*, num_identities + 1);
  GList *l;
  int idx = 0;
  for (l = identities; l != NULL; l = l->next) {
    PolkitIdentity *identity = POLKIT_IDENTITY(l->data);
    gchar *str = NULL;
    if (POLKIT_IS_UNIX_USER(identity)) {
      PolkitUnixUser *user = POLKIT_UNIX_USER(identity);
      const gchar *name = polkit_unix_user_get_name(user);
      if (name != NULL) {
        str = g_strdup(name);
      }
    }
    if (str == NULL) {
      str = polkit_identity_to_string(identity);
    }
    ident_strings[idx++] = str;
  }

  goInitiateAuthentication(
      at->id,
      (char*)(action_id ? action_id : ""),
      (char*)(message ? message : ""),
      (char*)(icon_name ? icon_name : ""),
      (char*)(cookie ? cookie : ""),
      ident_strings,
      num_identities
  );

  for (idx = 0; idx < num_identities; idx++) {
    g_free(ident_strings[idx]);
  }
  g_free(ident_strings);
}

static gboolean phalune_agent_listener_initiate_authentication_finish(
    PolkitAgentListener  *listener,
    GAsyncResult         *res,
    GError              **error)
{
  return g_task_propagate_boolean(G_TASK(res), error);
}

static void phalune_agent_listener_class_init(PhaluneAgentListenerClass *klass)
{
  PolkitAgentListenerClass *parent = POLKIT_AGENT_LISTENER_CLASS(klass);
  parent->initiate_authentication = phalune_agent_listener_initiate_authentication;
  parent->initiate_authentication_finish = phalune_agent_listener_initiate_authentication_finish;
}

static void phalune_agent_listener_init(PhaluneAgentListener *self)
{
}

PhaluneAgentListener *phalune_agent_listener_new(void)
{
  return g_object_new(PHALUNE_TYPE_AGENT_LISTENER, NULL);
}

void phalune_agent_task_complete(uintptr_t taskId, int success, const char *errMsg)
{
  if (active_tasks == NULL) return;

  AuthTask *at = (AuthTask*)g_hash_table_lookup(active_tasks, (gpointer)taskId);
  if (at == NULL) return;

  GTask *t = at->task;
  if (t != NULL) {
    if (success) {
      g_task_return_boolean(t, TRUE);
    } else {
      g_task_return_new_error(t,
                              POLKIT_ERROR,
                              POLKIT_ERROR_CANCELLED,
                              "%s",
                              errMsg ? errMsg : "Authentication cancelled");
    }
    g_object_unref(t);
    at->task = NULL;
  }

  g_hash_table_remove(active_tasks, (gpointer)taskId);
}

int phalune_agent_session_create(uintptr_t taskId, int identityIdx, const char *cookie)
{
  if (active_tasks == NULL) return 0;

  AuthTask *at = (AuthTask*)g_hash_table_lookup(active_tasks, (gpointer)taskId);
  if (at == NULL) return 0;

  auth_task_clean_session(at);

  PolkitIdentity *identity = NULL;
  if (at->identities != NULL) {
    identity = POLKIT_IDENTITY(g_list_nth_data(at->identities, identityIdx));
  }
  if (identity == NULL) return 0;

  at->session = polkit_agent_session_new(identity, cookie);
  if (at->session == NULL) return 0;

  at->req_handler = g_signal_connect(at->session, "request", G_CALLBACK(on_session_request), at);
  at->err_handler = g_signal_connect(at->session, "show-error", G_CALLBACK(on_session_show_error), at);
  at->info_handler = g_signal_connect(at->session, "show-info", G_CALLBACK(on_session_show_info), at);
  at->comp_handler = g_signal_connect(at->session, "completed", G_CALLBACK(on_session_completed), at);

  return 1;
}

void phalune_agent_session_initiate(uintptr_t taskId)
{
  if (active_tasks == NULL) return;
  AuthTask *at = (AuthTask*)g_hash_table_lookup(active_tasks, (gpointer)taskId);
  if (at != NULL && at->session != NULL) {
    polkit_agent_session_initiate(at->session);
  }
}

void phalune_agent_session_response(uintptr_t taskId, const char *response)
{
  if (active_tasks == NULL) return;
  AuthTask *at = (AuthTask*)g_hash_table_lookup(active_tasks, (gpointer)taskId);
  if (at != NULL && at->session != NULL) {
    polkit_agent_session_response(at->session, response ? response : "");
  }
}

void phalune_agent_session_cancel(uintptr_t taskId)
{
  if (active_tasks == NULL) return;
  AuthTask *at = (AuthTask*)g_hash_table_lookup(active_tasks, (gpointer)taskId);
  if (at != NULL && at->session != NULL) {
    polkit_agent_session_cancel(at->session);
  }
}

gpointer phalune_register_listener(PolkitAgentListener *listener, GError **error)
{
  PolkitSubject *subject = polkit_unix_session_new_for_process_sync(getpid(), NULL, NULL);
  if (subject == NULL) {
    subject = polkit_unix_process_new_for_owner(getpid(), 0, getuid());
  }

  gpointer handle = polkit_agent_listener_register(
      listener,
      POLKIT_AGENT_REGISTER_FLAGS_NONE,
      subject,
      "/org/phalune/PolicyKit1/AuthenticationAgent",
      NULL,
      error
  );

  g_object_unref(subject);
  return handle;
}

void phalune_unregister_listener(gpointer handle)
{
  if (handle != NULL) {
    polkit_agent_listener_unregister(handle);
  }
}
