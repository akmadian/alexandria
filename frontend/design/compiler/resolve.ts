// Token-source resolver (design-constitution §22, D29): reads tokens.resolver.json,
// layers its sets and theme-modifier contexts in resolutionOrder, and produces one
// flat, alias-free token map per theme. Pure data-in/data-out apart from loadSource.

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";

export interface AlxExtensions {
    alx?: {
        ink?: string;
        inset?: boolean;
        numerals?: string;
        role?: string;
        pin?: string;
        note?: string;
        inversion?: boolean;
    };
}

/** A raw node in a token file: a group (children) and/or a token ($value). */
export interface RawNode {
    $type?: string;
    $value?: unknown;
    $description?: string;
    $extensions?: AlxExtensions;
    [child: string]: unknown;
}

export interface ResolvedToken {
    path: string;
    type: string;
    value: unknown;
    extensions?: AlxExtensions;
    /** True when the winning value came from (or depends on) a theme-modifier layer. */
    varying: boolean;
    /** When the raw $value was a pure "{path}" alias: that path (e.g. accent → color.blue.solid). */
    aliasOf?: string;
}

export type ThemeTokens = Map<string, ResolvedToken>;

export interface ResolvedSource {
    themeNames: string[];
    defaultTheme: string;
    themes: Map<string, ThemeTokens>;
}

interface ResolverDocument {
    sets: Record<string, { sources: { $ref: string }[] }>;
    modifiers: {
        theme: { contexts: Record<string, { $ref: string }[]>; default: string };
    };
    resolutionOrder: { $ref: string }[];
}

/** One layered source: the parsed files of every base set plus one theme context. */
export interface SourceDocuments {
    themeNames: string[];
    defaultTheme: string;
    /** Per theme: token files in resolution order, tagged base (sets) or modifier. */
    layersByTheme: Map<string, { document: RawNode; modifier: boolean }[]>;
}

const RESERVED_KEYS = new Set(["$type", "$value", "$description", "$extensions", "$comment", "$schema"]);

export function loadSource(resolverPath: string): SourceDocuments {
    const resolverDirectory = dirname(resolverPath);
    const resolver = JSON.parse(readFileSync(resolverPath, "utf8")) as ResolverDocument;
    const readTokenFile = (reference: string): RawNode =>
        JSON.parse(readFileSync(join(resolverDirectory, reference), "utf8")) as RawNode;

    // resolutionOrder entries point at "#/sets/<name>" or "#/modifiers/theme".
    const baseLayers: RawNode[] = [];
    let sawThemeModifier = false;
    for (const entry of resolver.resolutionOrder) {
        const fragment = entry.$ref;
        const setMatch = /^#\/sets\/(.+)$/.exec(fragment);
        if (setMatch !== null) {
            const set = resolver.sets[setMatch[1]];
            if (set === undefined) throw new Error(`resolver: resolutionOrder references unknown set "${fragment}"`);
            for (const source of set.sources) baseLayers.push(readTokenFile(source.$ref));
            continue;
        }
        if (fragment === "#/modifiers/theme") {
            sawThemeModifier = true;
            continue;
        }
        throw new Error(`resolver: unsupported resolutionOrder reference "${fragment}"`);
    }
    if (!sawThemeModifier) throw new Error("resolver: resolutionOrder never applies the theme modifier");

    const themeNames = Object.keys(resolver.modifiers.theme.contexts);
    const layersByTheme = new Map<string, { document: RawNode; modifier: boolean }[]>();
    for (const themeName of themeNames) {
        const layers = baseLayers.map((document) => ({ document, modifier: false }));
        for (const source of resolver.modifiers.theme.contexts[themeName]) {
            layers.push({ document: readTokenFile(source.$ref), modifier: true });
        }
        layersByTheme.set(themeName, layers);
    }
    return { themeNames, defaultTheme: resolver.modifiers.theme.default, layersByTheme };
}

/** Pre-alias token: the winning raw $value plus which layer kind supplied it. */
interface FlatToken {
    path: string;
    type: string | undefined;
    value: unknown;
    extensions?: AlxExtensions;
    fromModifier: boolean;
}

function flattenLayer(
    document: RawNode,
    fromModifier: boolean,
    into: Map<string, FlatToken>,
    themeName: string,
): void {
    const walk = (node: RawNode, pathParts: string[], inheritedType: string | undefined): void => {
        const nodeType = typeof node.$type === "string" ? node.$type : inheritedType;
        if (node.$value !== undefined) {
            const path = pathParts.join(".");
            // A later layer's $value wins, but $type/$extensions fall back to the
            // earlier layer's when omitted — a theme override is a re-VALUING,
            // never a re-typing, and a conflicting re-type is a named failure.
            const previous = into.get(path);
            if (previous?.type !== undefined && nodeType !== undefined && previous.type !== nodeType) {
                throw new Error(
                    `${themeName}: token "${path}" re-typed across layers ("${previous.type}" → "${nodeType}") — an override re-values, never re-types`,
                );
            }
            into.set(path, {
                path,
                type: nodeType ?? previous?.type,
                value: node.$value,
                extensions: node.$extensions ?? previous?.extensions,
                fromModifier,
            });
        }
        for (const [key, child] of Object.entries(node)) {
            if (RESERVED_KEYS.has(key)) continue;
            if (child === null || typeof child !== "object" || Array.isArray(child)) continue;
            walk(child as RawNode, [...pathParts, key], nodeType);
        }
    };
    walk(document, [], undefined);
}

const ALIAS_PATTERN = /^\{([^{}]+)\}$/;

/** Resolve one theme's layers into an alias-free token map. Throws on broken references. */
export function resolveTheme(
    layers: { document: RawNode; modifier: boolean }[],
    themeName: string,
): ThemeTokens {
    const flat = new Map<string, FlatToken>();
    for (const layer of layers) flattenLayer(layer.document, layer.modifier, flat, themeName);

    const resolved = new Map<string, ResolvedToken>();
    const inProgress = new Set<string>();

    const resolveByPath = (path: string, requiredBy: string): ResolvedToken => {
        const existing = resolved.get(path);
        if (existing !== undefined) return existing;
        const token = flat.get(path);
        if (token === undefined) {
            throw new Error(`${themeName}: "{${path}}" referenced by "${requiredBy}" does not exist`);
        }
        if (inProgress.has(path)) {
            throw new Error(`${themeName}: alias cycle through "${path}"`);
        }
        inProgress.add(path);
        let varying = token.fromModifier;
        const resolveValue = (value: unknown): unknown => {
            if (typeof value === "string") {
                const aliasMatch = ALIAS_PATTERN.exec(value);
                if (aliasMatch === null) return value;
                const target = resolveByPath(aliasMatch[1], path);
                if (target.varying) varying = true;
                return target.value;
            }
            if (Array.isArray(value)) return value.map(resolveValue);
            if (value !== null && typeof value === "object") {
                return Object.fromEntries(
                    Object.entries(value as Record<string, unknown>).map(([key, entryValue]) => [
                        key,
                        resolveValue(entryValue),
                    ]),
                );
            }
            return value;
        };
        const value = resolveValue(token.value);
        inProgress.delete(path);
        if (token.type === undefined) {
            throw new Error(`${themeName}: token "${path}" has no $type (own or inherited)`);
        }
        const wholeAlias = typeof token.value === "string" ? ALIAS_PATTERN.exec(token.value) : null;
        const result: ResolvedToken = {
            path,
            type: token.type,
            value,
            extensions: token.extensions,
            varying,
            aliasOf: wholeAlias?.[1],
        };
        resolved.set(path, result);
        return result;
    };

    for (const path of flat.keys()) resolveByPath(path, "(root walk)");
    return resolved;
}

export function resolveSource(source: SourceDocuments): ResolvedSource {
    const themes = new Map<string, ThemeTokens>();
    for (const themeName of source.themeNames) {
        const layers = source.layersByTheme.get(themeName);
        if (layers === undefined) throw new Error(`no layers for theme "${themeName}"`);
        themes.set(themeName, resolveTheme(layers, themeName));
    }
    return { themeNames: source.themeNames, defaultTheme: source.defaultTheme, themes };
}
