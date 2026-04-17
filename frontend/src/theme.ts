import Aura from "@primeuix/themes/aura";
import { definePreset, palette, updatePreset } from "@primeuix/themes";

import type { ColorTheme } from "./types";

const themeSeeds: Record<ColorTheme, string> = {
    blue: "#355f96",
    graphite: "#627588",
    forest: "#2f7d69",
    sunset: "#c36f37",
    rose:    "#b84c6e",
    violet:  "#6b4fc8",
    amber:   "#a87928",
};

export const investGoPreset = definePreset(Aura, {
    semantic: {
        primary: palette(themeSeeds.blue),
    },
    components: {
        button: {
            root: {
                paddingX: "0.875rem",
                paddingY: "0.5rem",
                iconOnlyWidth: "2.25rem",
                sm: {
                    fontSize: "0.75rem",
                    paddingX: "0.625rem",
                    paddingY: "0.375rem",
                    iconOnlyWidth: "1.75rem",
                },
            },
        },
    },
});

export function applyPrimeVueColorTheme(colorTheme: ColorTheme): void {
    updatePreset({
        semantic: {
            primary: palette(themeSeeds[colorTheme]),
        },
    });
}
