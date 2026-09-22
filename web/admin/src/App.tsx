import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { RequireAuth } from './components/RequireAuth';
import { AdminLayout } from './components/AdminLayout';
import { Login } from './pages/Login';
import { Dashboard } from './pages/Dashboard';
import { Tasks } from './pages/Tasks';
import { Articles } from './pages/Articles';
import { ArticleEdit } from './pages/ArticleEdit';
import { RagIndex } from './pages/RagIndex';
import { AskUnansweredPage } from './pages/AskUnanswered';

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          element={
            <RequireAuth>
              <AdminLayout />
            </RequireAuth>
          }
        >
          <Route path="/" element={<Dashboard />} />
          <Route path="/tasks" element={<Tasks />} />
          <Route path="/articles" element={<Articles />} />
          <Route path="/articles/:id/edit" element={<ArticleEdit />} />
          <Route path="/rag-index" element={<RagIndex />} />
          <Route path="/ask-unanswered" element={<AskUnansweredPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
