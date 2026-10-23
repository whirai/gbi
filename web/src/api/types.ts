export interface MeUser {
  id: number
  username: string
  name: string
  role_code: string
  status: string
}

export interface User {
  id: number
  username: string
  name: string
  dept_id: number | null
  dept_name: string
  role_id: number
  role_code: string
  status: 'active' | 'disabled'
}

export interface Department {
  id: number
  parent_id: number | null
  name: string
  sort: number
  children?: Department[]
}

export interface Group {
  id: number
  name: string
  desc: string
}

export interface Role {
  id: number
  code: string
  name: string
}

export interface PageData<T> {
  items: T[]
  total: number
}

export interface Subject {
  id: number
  name: string
  code: string
  desc: string
  db_file: string
}
