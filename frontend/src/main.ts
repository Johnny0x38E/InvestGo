import { createApp } from "vue";
import PrimeVue from "primevue/config";

import App from "./App.vue";
import "./style.css";
import "./styles/forms.css";
import "./styles/tables.css";
import "./styles/overrides.css";
import "primeicons/primeicons.css";
import { investGoPreset } from "./theme";

const app = createApp(App);

app.use(PrimeVue, {
    theme: {
        preset: investGoPreset,
        options: {
            darkModeSelector: ".app-dark",
        },
    },
});

app.mount("#app");
