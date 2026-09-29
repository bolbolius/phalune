#ifndef PHALUNE_POLKIT_LISTENER_H
#define PHALUNE_POLKIT_LISTENER_H

#define POLKIT_AGENT_I_KNOW_API_IS_SUBJECT_TO_CHANGE 1
#include <polkitagent/polkitagent.h>
#include <glib.h>
#include <stdint.h>

G_BEGIN_DECLS

#define PHALUNE_TYPE_AGENT_LISTENER (phalune_agent_listener_get_type())
G_DECLARE_FINAL_TYPE(PhaluneAgentListener, phalune_agent_listener, PHALUNE, AGENT_LISTENER, PolkitAgentListener)

PhaluneAgentListener *phalune_agent_listener_new(void);

// Callbacks into Go (defined via //export in agent.go)
extern void goInitiateAuthentication(
    uintptr_t taskId,
    char *actionId,
    char *message,
    char *iconName,
    char *cookie,
    char **identities,
    int numIdentities
);
extern void goSessionRequest(uintptr_t taskId, char *request, int echoOn);
extern void goSessionShowError(uintptr_t taskId, char *text);
extern void goSessionShowInfo(uintptr_t taskId, char *text);
extern void goSessionCompleted(uintptr_t taskId, int gainedAuthorization);
extern void goTaskCancelled(uintptr_t taskId);

// C functions called from Go
void phalune_agent_task_complete(uintptr_t taskId, int success, const char *errMsg);

int phalune_agent_session_create(uintptr_t taskId, int identityIdx, const char *cookie);
void phalune_agent_session_initiate(uintptr_t taskId);
void phalune_agent_session_response(uintptr_t taskId, const char *response);
void phalune_agent_session_cancel(uintptr_t taskId);

gpointer phalune_register_listener(PolkitAgentListener *listener, GError **error);
void phalune_unregister_listener(gpointer handle);

G_END_DECLS

#endif /* PHALUNE_POLKIT_LISTENER_H */
