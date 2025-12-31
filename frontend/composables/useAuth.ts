export const useAuth = () => {
  const token = useState<string | null>("authToken", () => null);

  const loadToken = () => {
    if (!process.client || token.value) {
      return;
    }
    const stored = localStorage.getItem("authToken");
    token.value = stored && stored.trim() !== "" ? stored : null;
  };

  const setToken = (nextToken: string) => {
    token.value = nextToken;
    if (process.client) {
      localStorage.setItem("authToken", nextToken);
    }
  };

  const clearToken = () => {
    token.value = null;
    if (process.client) {
      localStorage.removeItem("authToken");
    }
  };

  return {
    token,
    loadToken,
    setToken,
    clearToken,
  };
};
