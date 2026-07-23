import js from "@eslint/js";
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactHooks from "eslint-plugin-react-hooks";
import storybook from "eslint-plugin-storybook";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
    // _generated-types is Go-generated (internal/seam/generate); tsc still
    // typechecks it (the completeness gate), but it is not hand-authored so it
    // is out of style lint. lib/pharos is the vendored DS WebGL web component
    // (a do-not-edit distributable, browser-global heavy) — same treatment.
    { ignores: ["dist", "node_modules", "coverage", "src/_generated-types", "src/lib/pharos"] },
    js.configs.recommended,
    ...tseslint.configs.recommended,
    jsxA11y.flatConfigs.recommended,
    {
        files: ["src/**/*.{ts,tsx}"],
        languageOptions: {
            globals: { ...globals.browser },
            // Type information for the type-aware rules below (exhaustive switches).
            parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
        },
        // react-hooks 7.1.1 ships its plugins key as a legacy string array, which
        // eslint 9 flat config rejects — wire the plugin object ourselves.
        plugins: { "react-hooks": reactHooks },
        rules: {
            ...reactHooks.configs["recommended-latest"].rules,
            // react-hooks 7 bundles React-Compiler rules. We keep the classics
            // (rules-of-hooks, exhaustive-deps) and set-state-in-effect (catches
            // real smells), but disable two compiler-oriented rules that fight
            // legitimate patterns when the compiler isn't running: refs (flags the
            // encapsulated imperative-DOM hook in the shell) and incompatible-library
            // (flags TanStack Virtual's returned functions). Re-enable if we adopt
            // the React Compiler.
            "react-hooks/refs": "off",
            "react-hooks/incompatible-library": "off",
            // Only api/ may touch a concrete backend impl; everyone else uses the
            // @/api/queries hooks. This is the one architecture rule cheap to enforce
            // with stock eslint. The feature-can't-import-feature rule (UI doc §14)
            // needs eslint-plugin-boundaries to express "other than self" — add it
            // if drift appears; until then the boundary is convention + review.
            "no-restricted-imports": [
                "error",
                { patterns: [{ group: ["**/api/mock-api", "**/api/wails-api"], message: "Import backend access from @/api/queries hooks, not the impl." }] },
            ],
            // Every switch over a generated union handles every member (C10) —
            // the lint-level twin of the `satisfies Record<Key, Entry>` gate,
            // covering void switches the return-type trick can't reach.
            // No default-clause escape hatch: a default would let a switch skip
            // members silently, which is the exact failure this rule exists for.
            "@typescript-eslint/switch-exhaustiveness-check": "error",
            // Hand-written parallel definitions are extinct (C15): declaring a
            // type whose name shadows a generated union is always a mistake —
            // import it from @/_generated-types instead.
            "no-restricted-syntax": [
                "error",
                {
                    selector:
                        "TSTypeAliasDeclaration[id.name=/^(TokenField|TokenOperator|ValueKind|ScopeKind|GroupOp|SortField|SortDir|FileType|ColorLabel|Flag|FileStatus|EventTopic|EventType|JobState|ApiErrorKind|ErrorCode)$/]",
                    message: "This union is generated — import it from @/_generated-types, never redeclare it (C13/C15).",
                },
            ],
        },
    },
    // Tests and the mock's own check touch internals freely.
    { files: ["src/**/*.test.{ts,tsx}", "src/test/**", "src/api/**"], rules: { "no-restricted-imports": "off" } },
    // Storybook's own best-practice rules for *.stories.* and .storybook/ config.
    ...storybook.configs["flat/recommended"],
);
