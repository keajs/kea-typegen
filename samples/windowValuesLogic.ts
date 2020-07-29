import {kea} from "kea";
import {windowValuesLogicType} from "./windowValuesLogicType";

export const windowValuesLogic = kea<windowValuesLogicType>({
    windowValues: () => ({
        windowHeight: (window) => window.innerHeight,
        windowWidth: (window) => Math.min(window.innerWidth, window.document.body.clientWidth),
    }),
})
