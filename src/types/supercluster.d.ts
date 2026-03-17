declare module "supercluster" {
  export interface SuperclusterOptions {
    minZoom?: number;
    maxZoom?: number;
    minPoints?: number;
    radius?: number;
    extent?: number;
    nodeSize?: number;
    log?: boolean;
    generateId?: boolean;
  }

  export default class Supercluster<TProps = Record<string, unknown>> {
    constructor(options?: SuperclusterOptions);
    load(points: Array<Record<string, unknown>>): this;
    getClusters(bbox: [number, number, number, number], zoom: number): Array<Record<string, unknown>>;
  }
}
