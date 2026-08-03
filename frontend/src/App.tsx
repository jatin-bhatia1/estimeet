import { Navigate, Route, Routes } from 'react-router-dom'

import { AppFooter } from './components/AppFooter'
import HomePage from './pages/HomePage'
import RoomPage from './pages/RoomPage'

export default function App() {
  // The column layout keeps the footer at the end of the content on a long
  // board, and at the bottom of the window on a short one.
  return (
    <div className="flex min-h-screen flex-col">
      <div className="flex-1">
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/room/:code" element={<RoomPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </div>
      <AppFooter />
    </div>
  )
}
