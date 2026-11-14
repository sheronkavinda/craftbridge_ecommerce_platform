import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
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
    ]);
    adminServiceSpy.listUsers.and.returnValue(of(mockUsers));
    adminServiceSpy.updateUserStatus.and.returnValue(
      of({ ...mockUsers[1], status: 'SUSPENDED' }),
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

  it('should display the empty state when no users match filters', () => {
    component.onSearchChange('nonexistent-email@example.com');
    fixture.detectChanges();

    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('No users match your filters');
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

  it('should protect ADMIN users from being suspended', () => {
    expect(component.isProtectedUser(mockUsers[0])).toBeTrue();
    component.requestSuspend(mockUsers[0]);
    expect(component.pendingSuspendUser()).toBeNull();
  });
});
