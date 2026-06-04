'use client'

import { useState, useEffect, FormEvent } from 'react'
import { useRouter } from 'next/navigation'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { useToast } from '@/components/ui/toaster'
import { loginWithKey, verifySession } from '@/lib/auth'
import { ArrowRight, ShieldCheck } from 'lucide-react'

export default function LoginPage() {
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [checking, setChecking] = useState(true)
  const router = useRouter()
  const { toast } = useToast()

  // Auto-verify existing session cookie on mount
  useEffect(() => {
    verifySession()
      .then((valid) => {
        if (valid) {
          router.replace('/dashboard/')
        } else {
          setChecking(false)
        }
      })
      .catch(() => setChecking(false))
  }, [router])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!password.trim() || loading) return

    setLoading(true)
    try {
      const valid = await loginWithKey(password.trim())
      if (valid) {
        router.replace('/dashboard/')
      } else {
        toast({ title: 'Invalid password', variant: 'destructive' })
      }
    } catch {
      toast({ title: 'Connection failed', variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }

  if (checking) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-muted">Loading...</div>
      </div>
    )
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-background px-4">
      <div className="pointer-events-none absolute inset-0">
        <div className="absolute -left-20 top-10 h-72 w-72 rounded-full bg-[#005FB8]/15 blur-3xl" />
        <div className="absolute right-[-90px] bottom-[-40px] h-96 w-96 rounded-full bg-[#00A3FF]/10 blur-3xl" />
      </div>

      <form onSubmit={handleSubmit} className="relative w-full max-w-[400px]">
        <Card className="fluent-card w-full border-border/80">
          <CardHeader className="text-center">
            <div className="mx-auto mb-2 flex h-11 w-11 items-center justify-center rounded-xl bg-primary/15 text-primary">
              <ShieldCheck className="h-5 w-5" />
            </div>
            <CardTitle className="text-2xl">Welcome Back</CardTitle>
            <CardDescription>Sign in to continue to MasantoID Admin</CardDescription>
          </CardHeader>
          <CardContent>
            <Input
              type="password"
              placeholder="Admin password (app_key)"
              aria-label="Admin password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoFocus
            />
          </CardContent>
          <CardFooter>
            <Button type="submit" className="w-full" disabled={loading || !password.trim()}>
              {loading ? 'Signing in...' : 'Sign In'}
              {!loading && <ArrowRight className="h-4 w-4" />}
            </Button>
          </CardFooter>
        </Card>
      </form>
    </div>
  )
}
