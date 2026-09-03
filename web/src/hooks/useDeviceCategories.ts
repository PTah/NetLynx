import { useCallback, useEffect, useState } from "react";
import { apiGet } from "../api";
import {
  BUILTIN_DEVICE_CATEGORIES,
  type DeviceCategoryDef,
} from "../deviceCategories";

type ListResp = { categories?: DeviceCategoryDef[] };

let cache: DeviceCategoryDef[] | null = null;
let inflight: Promise<DeviceCategoryDef[]> | null = null;
const listeners = new Set<() => void>();

function notify() {
  for (const l of listeners) l();
}

async function fetchCategories(): Promise<DeviceCategoryDef[]> {
  try {
    const r = await apiGet<ListResp>("/api/v1/settings/device-categories");
    const list = Array.isArray(r?.categories) ? r.categories : [];
    if (list.length > 0) return list;
  } catch {
    /* offline / старый сервер */
  }
  return BUILTIN_DEVICE_CATEGORIES.map((c) => ({ ...c }));
}

export function invalidateDeviceCategoriesCache(): void {
  cache = null;
  inflight = null;
  notify();
}

export function useDeviceCategories(): {
  categories: DeviceCategoryDef[];
  loading: boolean;
  reload: () => void;
} {
  const [categories, setCategories] = useState<DeviceCategoryDef[]>(
    () => cache ?? BUILTIN_DEVICE_CATEGORIES,
  );
  const [loading, setLoading] = useState(cache == null);

  const load = useCallback(() => {
    setLoading(true);
    if (!inflight) {
      inflight = fetchCategories().then((list) => {
        cache = list;
        inflight = null;
        notify();
        return list;
      });
    }
    inflight
      .then((list) => setCategories(list))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    const onChange = () => {
      if (cache) setCategories(cache);
    };
    listeners.add(onChange);
    load();
    return () => {
      listeners.delete(onChange);
    };
  }, [load]);

  const reload = useCallback(() => {
    invalidateDeviceCategoriesCache();
    load();
  }, [load]);

  return { categories: categories ?? BUILTIN_DEVICE_CATEGORIES, loading, reload };
}
