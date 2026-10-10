import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { RequireAuth } from './components/RequireAuth';
import { AdminLayout } from './components/AdminLayout';
import { APP_ROUTES, PUBLIC_ROUTES } from './router';

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        {PUBLIC_ROUTES.map(({ path, element }) => (
          <Route key={path} path={path} element={element} />
        ))}
        <Route
          element={
            <RequireAuth>
              <AdminLayout />
            </RequireAuth>
          }
        >
          {APP_ROUTES.map(({ path, element }) => (
            <Route key={path} path={path} element={element} />
          ))}
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
