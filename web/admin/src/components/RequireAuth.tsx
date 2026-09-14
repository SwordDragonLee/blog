import { Navigate, useLocation } from 'react-router-dom';
import type { ReactNode } from 'react';
import { observer } from 'mobx-react-lite';
import { authStore } from '../stores';

export const RequireAuth = observer(function RequireAuth({
  children,
}: {
  children: ReactNode;
}) {
  const location = useLocation();
  if (!authStore.isAuthenticated) {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />;
  }
  return children;
});
