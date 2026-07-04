export interface Collection {
    id: string;
    name: string;
    kind: "manual" | "smart";
    count: number;
}
