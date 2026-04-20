import { ref } from "vue";
import { translate } from "../i18n";

export function useConfirmDialog(
    onDeleteItem: (id: string) => Promise<void>,
    onDeleteAlert: (id: string) => Promise<void>,
) {
    const confirmDialogVisible = ref(false);
    const confirmTitle = ref("");
    const confirmMessage = ref("");
    const confirmLabel = ref(translate("common.delete"));
    const deleting = ref(false);
    const pendingDelete = { kind: "" as "" | "item" | "alert", id: "" };

    // Record the pending delete target and open the confirmation dialog.
    function requestDeleteItem(id: string): void {
        pendingDelete.kind = "item";
        pendingDelete.id = id;
        confirmTitle.value = translate("dialogs.confirm.deleteItemTitle");
        confirmMessage.value = translate("dialogs.confirm.deleteItemMessage");
        confirmLabel.value = translate("dialogs.confirm.deleteItemLabel");
        confirmDialogVisible.value = true;
    }

    function requestDeleteAlert(id: string): void {
        pendingDelete.kind = "alert";
        pendingDelete.id = id;
        confirmTitle.value = translate("dialogs.confirm.deleteAlertTitle");
        confirmMessage.value = translate("dialogs.confirm.deleteAlertMessage");
        confirmLabel.value = translate("dialogs.confirm.deleteAlertLabel");
        confirmDialogVisible.value = true;
    }

    // Execute the confirmed delete action.
    async function confirmDelete(): Promise<void> {
        if (!pendingDelete.kind || !pendingDelete.id) {
            confirmDialogVisible.value = false;
            return;
        }

        deleting.value = true;
        try {
            // The delete target has already been frozen into pendingDelete before
            // confirmation, so this branch only performs the matching action.
            if (pendingDelete.kind === "item") {
                await onDeleteItem(pendingDelete.id);
            } else {
                await onDeleteAlert(pendingDelete.id);
            }
            confirmDialogVisible.value = false;
        } finally {
            deleting.value = false;
            pendingDelete.kind = "";
            pendingDelete.id = "";
        }
    }

    return {
        confirmDialogVisible,
        confirmTitle,
        confirmMessage,
        confirmLabel,
        deleting,
        requestDeleteItem,
        requestDeleteAlert,
        confirmDelete,
    };
}
