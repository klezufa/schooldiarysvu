import { navigationFor } from '../lib/navigation'
import { userFixture } from '../test/fixtures'

describe('role navigation', () => {
  it('gives teachers journal but not student grades or administration', () => {
    const paths = navigationFor({ ...userFixture, roles: ['teacher'] }).map((item) => item.to)
    expect(paths).toContain('/journal')
    expect(paths).toContain('/reports')
    expect(paths).not.toContain('/grades')
    expect(paths).not.toContain('/admin')
  })

  it('gives parents student sections without teacher journal', () => {
    const paths = navigationFor({ ...userFixture, roles: ['parent'], student_id: undefined }).map((item) => item.to)
    expect(paths).toContain('/grades')
    expect(paths).toContain('/homework')
    expect(paths).not.toContain('/journal')
  })

  it('shows administration to the system administrator too', () => {
    const paths = navigationFor({ ...userFixture, roles: ['system_admin'] }).map((item) => item.to)
    expect(paths).toContain('/admin')
  })

  it('gives class teacher own class and administrator only allowed mobile priorities', () => {
    expect(navigationFor({ ...userFixture, roles: ['class_teacher'] }).map((item) => item.to)).toContain('/my-class')
    const admin = navigationFor({ ...userFixture, roles: ['administrator'] })
    expect(admin.map((item) => item.to)).toContain('/admin')
    expect(admin.filter((item) => item.main).length).toBeLessThanOrEqual(4)
  })
})
