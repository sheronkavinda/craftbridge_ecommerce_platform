import { ComponentFixture, TestBed, fakeAsync, tick } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { UserSummary } from '../admin.models';
import { AdminService } from '../admin.service';
import { UsersComponent } from './users.component';

describe('UsersComponent', () => {
  let component: UsersComponent;
  let fixture: ComponentFixture<UsersComponent>;
  let adminServiceSpy: jasmine.SpyObj<AdminService>;

  const mockUsers: UserSummary[] = [
    {
      id: 'admin-001',
      name: 'System Admin',
      email: 'admin@craftbridge.test',
      role: 'ADMIN',
      status: 'ACTIVE',
      created_at: '2026-11-01T10:00:00Z',
    },
    {
      id: 'buyer-001',
      name: 'Nimal Perera',
      email: 'nimal@craftbridge.test',
      role: 'BUYER',
      status: 'ACTIVE',
      created_at: '2026-11-02T10:00:00Z',
    },
    {
      id: 'buyer-003',
      name: 'Ruwan Fernando',
      email: 'ruwan@craftbridge.test',
      role: 'BUYER',
      status: 'SUSPENDED',
      created_at: '2026-11-03T10:00:00Z',
    },
  ];

  beforeEach(async () => {
    adminServiceSpy = jasmine.createSpyObj<AdminService>('AdminService', [
      'listUsers',
      'updateUserStatus',
      'updateUserRole',
    ]);
    adminServiceSpy.listUsers.and.returnValue(of(mockUsers));
    adminServiceSpy.updateUserStatus.and.returnValue(
      of({ ...mockUsers[1], status: 'SUSPENDED' }),
    );
    adminServiceSpy.updateUserRole.and.returnValue(
      of({ ...mockUsers[1], role: 'SELLER' }),
    );

    await TestBed.configureTestingModule({
      imports: [UsersComponent],
      providers: [{ provide: AdminService, useValue: adminServiceSpy }],
    }).compileComponents();

    fixture = TestBed.createComponent(UsersComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should render the user list from the mocked service', () => {
    const compiled = fixture.nativeElement as HTMLElement;
    const rows = compiled.querySelectorAll('tbody tr');
    expect(rows.length).toBe(3);
    expect(rows[0].textContent).toContain('System Admin');
    expect(rows[1].textContent).toContain('Nimal Perera');
    expect(rows[2].textContent).toContain('Ruwan Fernando');
  });

  it('should debounce server-side search by 300ms and pass trimmed q to listUsers', fakeAsync(() => {
    adminServiceSpy.listUsers.calls.reset();
    adminServiceSpy.listUsers.and.returnValue(of([mockUsers[1]]));

    component.onSearchChange('  nimal@craftbridge.test  ');
    tick(200);
    expect(adminServiceSpy.listUsers).not.toHaveBeenCalled();

    tick(100);
    expect(adminServiceSpy.listUsers).toHaveBeenCalledTimes(1);
    expect(adminServiceSpy.listUsers).toHaveBeenCalledWith('', '', 'nimal@craftbridge.test');
  }));

  it('should open the role-change confirm dialog and update the row on confirm', () => {
    component.requestRoleChange(mockUsers[1], 'SELLER');
    fixture.detectChanges();

    const dialog = (fixture.nativeElement as HTMLElement).querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain('Change Nimal Perera from BUYER to SELLER?');
    expect(adminServiceSpy.updateUserRole).not.toHaveBeenCalled();

    component.confirmRoleChange();
    fixture.detectChanges();

    expect(adminServiceSpy.updateUserRole).toHaveBeenCalledWith('buyer-001', {
      role: 'SELLER',
    });
    expect(component.users()[1].role).toBe('SELLER');
    expect(component.successMessage()).toContain('Role for "Nimal Perera" changed from BUYER to SELLER');
  });

  it('should display backend error message when role update returns 409', () => {
    adminServiceSpy.updateUserRole.and.returnValue(
      throwError(() => ({ error: { error: 'cannot remove the last administrator' } })),
    );

    const secondAdmin: UserSummary = {
      id: 'admin-002',
      name: 'Second Admin',
      email: 'admin2@craftbridge.test',
      role: 'ADMIN',
      status: 'ACTIVE',
      created_at: '2026-11-01T10:00:00Z',
    };

    component.requestRoleChange(secondAdmin, 'BUYER');
    component.confirmRoleChange();

    expect(component.actionError()).toBe('cannot remove the last administrator');
  });

  it('should open the confirm dialog before suspending an active user', () => {
    component.requestSuspend(mockUsers[1]);
    fixture.detectChanges();

    const dialog = (fixture.nativeElement as HTMLElement).querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain('Suspend Nimal Perera?');
    expect(adminServiceSpy.updateUserStatus).not.toHaveBeenCalled();

    component.confirmSuspend();
    expect(adminServiceSpy.updateUserStatus).toHaveBeenCalledWith('buyer-001', {
      status: 'SUSPENDED',
    });
  });

  it('should protect ADMIN users from being suspended and prevent self-role changes', () => {
    expect(component.isProtectedUser(mockUsers[0])).toBeTrue();
    component.requestSuspend(mockUsers[0]);
    expect(component.pendingSuspendUser()).toBeNull();

    component.requestRoleChange(mockUsers[0], 'BUYER');
    expect(component.pendingRoleChange()).toBeNull();
  });
});
