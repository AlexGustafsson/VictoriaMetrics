import { useCallback, useMemo, useRef, useState } from "preact/compat";
import { getLogHitsUrl } from "../../../api/logs";
import { ErrorTypes, TimeParams } from "../../../types";
import { LogHits } from "../../../api/types";
import { useSearchParams } from "react-router-dom";
import { getHitsTimeParams } from "../../../utils/logs";

export const HITS_GROUP_FIELD = "_stream";  // In the future, this field can be made configurable

export const useFetchLogHits = (server: string, query: string) => {
  const [searchParams] = useSearchParams();

  const [logHits, setLogHits] = useState<LogHits[]>([]);
  const [otherHits, setOtherHits] = useState<LogHits[]>([]);
  const [isLoading, setIsLoading] = useState<{[key: number]: boolean;}>([]);
  const [error, setError] = useState<ErrorTypes | string>();
  const abortControllerRef = useRef(new AbortController());

  const url = useMemo(() => getLogHitsUrl(server), [server]);

  const getOptions = (query: string, period: TimeParams, signal: AbortSignal) => {
    const { start, end, step } = getHitsTimeParams(period);

    return {
      signal,
      method: "POST",
      headers: {
        AccountID: searchParams.get("accountID") || "0",
        ProjectID: searchParams.get("projectID") || "0",
      },
      body: new URLSearchParams({
        query: query.trim(),
        step: `${step}ms`,
        start: start.toISOString(),
        end: end.toISOString(),
        field: HITS_GROUP_FIELD,

      })
    };
  };

  const accumulateHits = (resultHit: LogHits, hit: LogHits) => {
    resultHit.total = (resultHit.total || 0) + (hit.total || 0);
    hit.timestamps.forEach((timestamp, i) => {
      const index = resultHit.timestamps.findIndex(t => t === timestamp);
      if (index === -1) {
        resultHit.timestamps.push(timestamp);
        resultHit.values.push(hit.values[i]);
      } else {
        resultHit.values[index] += hit.values[i];
      }
    });
    return resultHit;
  };

  const getHitsWithTop = (hits: LogHits[]) => {
    const topN = 5;
    const defaultHit: LogHits = { fields: {}, timestamps: [], values: [], total: 0, _isOther: true };

    const hitsByTotal = hits.sort((a, b) => (b.total || 0) - (a.total || 0));
    const result = [];

    const otherHits = hitsByTotal.slice(topN);
    const otherHitsAccumulated: LogHits = otherHits.reduce(accumulateHits, defaultHit);
    if (otherHitsAccumulated.total) {
      result.push(otherHitsAccumulated);
    }

    const topHits: LogHits[] = hitsByTotal.slice(0, topN);
    if (topHits.length) {
      result.push(...topHits);
    }

    return { result, otherHits };
  };

  const fetchLogHits = useCallback(async (period: TimeParams) => {
    abortControllerRef.current.abort();
    abortControllerRef.current = new AbortController();
    const { signal } = abortControllerRef.current;

    const id = Date.now();
    setIsLoading(prev => ({ ...prev, [id]: true }));
    setError(undefined);

    try {
      const options = getOptions(query, period, signal);
      const response = await fetch(url, options);

      if (!response.ok || !response.body) {
        const text = await response.text();
        setError(text);
        setLogHits([]);
        setOtherHits([]);
        setIsLoading(prev => ({ ...prev, [id]: false }));
        return;
      }

      const data = await response.json();
      const hits = data?.hits as LogHits[];
      if (!hits) {
        const error = "Error: No 'hits' field in response";
        setError(error);
      }

      const { result = [], otherHits = [] } = !hits ? {} : getHitsWithTop(hits);
      setLogHits(result);
      setOtherHits(otherHits);
    } catch (e) {
      if (e instanceof Error && e.name !== "AbortError") {
        setError(String(e));
        console.error(e);
        setLogHits([]);
        setOtherHits([]);
      }
    }
    setIsLoading(prev => ({ ...prev, [id]: false }));
  }, [url, query, searchParams]);

  return {
    logHits,
    otherHits,
    isLoading: Object.values(isLoading).some(s => s),
    error,
    fetchLogHits,
    abortController: abortControllerRef.current
  };
};
