import { reactive, ref } from "vue";
import { api } from "../api";
import { emptyAlertForm, mapAlertToForm } from "../forms";
import { translate } from "../i18n";
import type { AlertFormModel, AlertRule, StateSnapshot, StatusTone } from "../types";

type StatusReporter = (message: string, tone: StatusTone) => void;

export function useAlertDialog(
    applySnapshot: (snapshot: StateSnapshot) => void,
    setStatus: StatusReporter,
    onAlertSaved: () => void,
) {
    const alertDialogVisible = ref(false);
    const savingAlert = ref(false);
    const alertForm = reactive<AlertFormModel>(emptyAlertForm());

    // Open the alert editor dialog.
    // When no alert is provided, defaultItemId seeds the new alert's itemId.
    function openAlertDialog(alert?: AlertRule, defaultItemId = ""): void {
        Object.assign(
            alertForm,
            alert ? mapAlertToForm(alert) : emptyAlertForm(defaultItemId),
        );
        alertDialogVisible.value = true;
    }

    // Save the alert rule and invoke onAlertSaved so the caller can switch the UI.
    async function saveAlert(): Promise<void> {
        savingAlert.value = true;
        try {
            const path = alertForm.id
                ? `/api/alerts/${alertForm.id}`
                : "/api/alerts";
            const method = alertForm.id ? "PUT" : "POST";
            const snapshot = await api<StateSnapshot>(path, {
                method,
                body: JSON.stringify(alertForm),
            });
            applySnapshot(snapshot);
            alertDialogVisible.value = false;
            setStatus(
                alertForm.id
                    ? translate("app.alertUpdated")
                    : translate("app.alertAdded"),
                "success",
            );
            onAlertSaved();
        } catch (error) {
            setStatus(
                error instanceof Error
                    ? error.message
                    : translate("app.alertSaveFailed"),
                "error",
            );
        } finally {
            savingAlert.value = false;
        }
    }

    // Delete an alert rule.
    async function performDeleteAlert(id: string): Promise<void> {
        try {
            const snapshot = await api<StateSnapshot>(`/api/alerts/${id}`, {
                method: "DELETE",
            });
            applySnapshot(snapshot);
            setStatus(translate("app.alertDeleted"), "success");
        } catch (error) {
            setStatus(
                error instanceof Error
                    ? error.message
                    : translate("app.deleteFailed"),
                "error",
            );
        }
    }

    return {
        alertDialogVisible,
        savingAlert,
        alertForm,
        openAlertDialog,
        saveAlert,
        performDeleteAlert,
    };
}
