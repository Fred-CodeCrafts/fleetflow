import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './shell/Layout'
import { allNavItems, navigation } from './shell/navigation'
import { Screen } from './screens/Screen'
import './styles.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/operations" replace />} />
          {navigation.flatMap((branch) => branch.children.map((item) => (
            <Route key={item.id} path={item.path} element={<Screen id={item.id} />} />
          )))}
          {navigation.filter((branch) => !allNavItems().some((item) => item.path === branch.path)).map((branch) => (
            <Route key={branch.id} path={branch.path} element={<Navigate to={branch.children[0]?.path ?? '/operations'} replace />} />
          ))}
          <Route path="*" element={<Navigate to="/operations" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  </React.StrictMode>,
)
